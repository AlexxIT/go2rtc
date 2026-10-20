package reolink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Producer struct {
	core.Connection

	profile *Profile
	ctx     context.Context
	cancel  context.CancelFunc
	stop    sync.Once
	stopErr error
	send    atomic.Uint64

	failureMu sync.Mutex
	failure   error
	talkMu    sync.Mutex
	talk      *backchannel
}

func (p *Producer) Format(state fmt.State, _ rune) {
	profile := p.profile
	if profile == nil {
		_, _ = fmt.Fprintf(state, "reolink.Producer{ID:%d}", p.ID)
		return
	}
	_, _ = fmt.Fprintf(state, "reolink.Producer{ID:%d, Remote:%q, Channel:%d, Stream:%q, Generation:%d}",
		p.ID, p.RemoteAddr, profile.key.Channel, profile.key.Stream, profile.generation)
}

var _ core.Producer = (*Producer)(nil)
var _ core.Consumer = (*Producer)(nil)

func newProducer(s source, profile *Profile, talkFormat *baichuan.TalkFormat) *Producer {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Producer{
		Connection: core.Connection{
			ID: core.NewID(), FormatName: "baichuan", Protocol: s.protocol, RemoteAddr: s.remote,
		},
		profile: profile,
		ctx:     ctx, cancel: cancel,
	}
	p.Medias = append(p.Medias, profile.pipeline.medias...)
	p.Receivers = append(p.Receivers, profile.receivers...)
	if talkFormat != nil {
		p.talk = &backchannel{producer: p, format: *talkFormat}
		p.Medias = append(p.Medias, &core.Media{
			Kind: core.KindAudio, Direction: core.DirectionSendonly,
			Codecs: talkCodecs(),
		})
	}
	return p
}

func (p *Producer) Start() error {
	p.profile.startProfile()
	select {
	case <-p.profile.done:
		if err := p.ctx.Err(); err != nil {
			return p.terminalError(err)
		}
		return p.terminalError(p.profile.err())
	case <-p.ctx.Done():
		<-p.profile.done
		return p.terminalError(p.ctx.Err())
	}
}

func (p *Producer) fail(err error) {
	if err == nil {
		return
	}
	p.failureMu.Lock()
	if p.failure == nil {
		p.failure = err
		p.cancel()
		if p.profile != nil {
			p.profile.cancel()
		}
	}
	p.failureMu.Unlock()
}

func (p *Producer) terminalError(fallback error) error {
	p.failureMu.Lock()
	defer p.failureMu.Unlock()
	if p.failure != nil {
		return p.failure
	}
	if fallback != nil {
		return fallback
	}
	return errors.New("reolink: profile stopped")
}

func (p *Producer) Stop() error {
	p.stop.Do(func() {
		p.cancel()
		p.talkMu.Lock()
		if p.talk != nil {
			p.stopErr = errors.Join(p.stopErr, p.talk.Close())
		}
		p.talkMu.Unlock()
		p.profile.camera.registry.release(p.profile)
		<-p.profile.done
		p.stopErr = errors.Join(p.stopErr, p.profile.closeError())
		p.profile.closeReceivers()
	})
	return p.stopErr
}

func (p *Producer) addSend(size int) {
	p.send.Add(uint64(size))
}

type producerDiagnostics struct {
	ID         uint32             `json:"id,omitempty"`
	FormatName string             `json:"format_name,omitempty"`
	Protocol   string             `json:"protocol,omitempty"`
	RemoteAddr string             `json:"remote_addr,omitempty"`
	Medias     []*core.Media      `json:"medias,omitempty"`
	Recv       uint64             `json:"bytes_recv,omitempty"`
	Send       uint64             `json:"bytes_send,omitempty"`
	Reolink    profileDiagnostics `json:"reolink"`
}

type profileDiagnostics struct {
	Channel         uint8    `json:"channel"`
	Profile         string   `json:"profile"`
	State           string   `json:"state"`
	Generation      uint64   `json:"generation"`
	Profiles        int      `json:"profiles"`
	MainSessions    int      `json:"main_sessions"`
	OtherSessions   int      `json:"other_sessions"`
	VideoCodec      string   `json:"video_codec,omitempty"`
	AudioCodec      string   `json:"audio_codec,omitempty"`
	CameraType      string   `json:"camera_type,omitempty"`
	CameraModel     string   `json:"camera_model,omitempty"`
	HardwareVersion string   `json:"hardware_version,omitempty"`
	FirmwareVersion string   `json:"firmware_version,omitempty"`
	DeviceInfo      string   `json:"device_info_status,omitempty"`
	CapabilityInfo  string   `json:"capability_status,omitempty"`
	Channels        []uint16 `json:"observed_channels,omitempty"`
	LastError       string   `json:"last_error_class,omitempty"`
}

func (p *Producer) diagnostics() profileDiagnostics {
	profile := p.profile
	registry := profile.camera.registry
	p.failureMu.Lock()
	failureClass := errorClass(p.failure)
	p.failureMu.Unlock()
	registry.mu.Lock()
	profile.mu.Lock()
	channels := make([]uint16, len(profile.discovery.capabilities.ObservedChannels))
	for i, channel := range profile.discovery.capabilities.ObservedChannels {
		channels[i] = uint16(channel)
	}
	d := profileDiagnostics{
		Channel: profile.key.Channel, Profile: string(profile.key.Stream),
		State: profile.state.String(), Generation: profile.generation, Profiles: len(profile.camera.profiles),
		MainSessions: profile.camera.mainSessions, OtherSessions: profile.camera.otherSessions,
		VideoCodec: profile.pipeline.videoCodec,
		CameraType: profile.discovery.device.Type, CameraModel: profile.discovery.device.Model,
		HardwareVersion: profile.discovery.device.Hardware, FirmwareVersion: profile.discovery.device.Firmware,
		DeviceInfo: profile.discovery.deviceStatus, CapabilityInfo: profile.discovery.capabilityStatus,
		Channels:  channels,
		LastError: profile.errClass,
	}
	if profile.pipeline.audioMedia != nil {
		d.AudioCodec = profile.pipeline.audioMedia.Codecs[0].Name
	}
	if failureClass != "" {
		d.LastError = failureClass
	}
	profile.mu.Unlock()
	registry.mu.Unlock()
	return d
}

func (p *Producer) MarshalJSON() ([]byte, error) {
	return json.Marshal(&producerDiagnostics{
		ID: p.ID, FormatName: p.FormatName, Protocol: p.Protocol, RemoteAddr: p.RemoteAddr,
		Medias: p.Medias, Recv: p.profile.recvBytes.Load(), Send: p.send.Load(), Reolink: p.diagnostics(),
	})
}
