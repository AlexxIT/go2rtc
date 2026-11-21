package reolink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type producerFormat struct {
	connection *core.Connection
	key        ProfileKey
	generation uint64
}

func (f producerFormat) Format(state fmt.State, _ rune) {
	if f.connection == nil {
		_, _ = fmt.Fprint(state, "reolink.Producer{ID:0}")
		return
	}
	_, _ = fmt.Fprintf(state, "reolink.Producer{ID:%d, Remote:%q, Channel:%d, Stream:%q, Generation:%d}",
		f.connection.ID, f.connection.RemoteAddr, f.key.Channel, f.key.Stream, f.generation)
}

var _ core.Producer = (*Producer)(nil)
var _ core.Consumer = (*Producer)(nil)
var _ fmt.Formatter = Producer{}
var _ fmt.Formatter = (*Producer)(nil)

func (p *Producer) Start() error {
	p.start.Do(p.startCancel)
	select {
	case <-p.done:
		if err := p.stopCtx.Err(); err != nil {
			return err
		}
		p.waitRetry()
		if err := p.stopCtx.Err(); err != nil {
			return err
		}
		return p.err()
	case <-p.stopCtx.Done():
		<-p.done
		return p.stopCtx.Err()
	}
}

func (p *Producer) waitRetry() {
	if remaining := time.Until(p.attemptBegan.Add(profileRetryDelay)); remaining > 0 {
		timer := time.NewTimer(remaining)
		defer timer.Stop()
		var stop <-chan struct{}
		if p.stopCtx != nil {
			stop = p.stopCtx.Done()
		}
		select {
		case <-timer.C:
		case <-stop:
		}
	}
}

func (p *Producer) Stop() error {
	p.stop.Do(func() {
		p.stopCancel()
		p.mu.Lock()
		input := p.input
		p.mu.Unlock()
		aborted := make(chan struct{})
		var abortErr error
		timer := time.AfterFunc(2*time.Second, func() {
			defer close(aborted)
			p.camera.registry.release(p)
			if input != nil {
				abortErr = input.Abort()
			}
		})
		defer func() {
			if !timer.Stop() {
				<-aborted
				p.stopErr = errors.Join(p.stopErr, abortErr, context.DeadlineExceeded)
				p.camera.registry.log.Warn().Str("error_class", "timeout").Str("camera", p.camera.remote).
					Uint8("channel", p.key.Channel).Str("profile", string(p.key.Stream)).
					Msg("reolink shutdown grace expired")
			}
		}()
		p.talkMu.Lock()
		if p.talk != nil {
			p.stopErr = errors.Join(p.stopErr, p.talk.Close())
		}
		p.talkMu.Unlock()
		p.camera.registry.release(p)
		<-p.done
		p.stopErr = errors.Join(p.stopErr, p.closeError())
		p.closeReceivers()
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
	BoundaryAdjust  uint64   `json:"boundary_adjustments"`
	VideoShort      uint64   `json:"short_video_units"`
	VideoRecoveries uint64   `json:"video_resyncs"`
	RefetchAttempts uint64   `json:"refetch_attempts,omitempty"`
	RefetchResumes  uint64   `json:"refetch_resumes,omitempty"`
	RefetchEnabled  bool     `json:"refetch_enabled,omitempty"`
	LastError       string   `json:"last_error_class,omitempty"`
}

func (p *Producer) diagnostics() profileDiagnostics {
	registry := p.camera.registry
	registry.mu.Lock()
	p.mu.Lock()
	input := p.input
	channels := make([]uint16, len(p.discovery.capabilities.ObservedChannels))
	for i, channel := range p.discovery.capabilities.ObservedChannels {
		channels[i] = uint16(channel)
	}
	d := profileDiagnostics{
		Channel: p.key.Channel, Profile: string(p.key.Stream),
		State: p.state.String(), Generation: p.generation, Profiles: len(p.camera.profiles),
		MainSessions: p.camera.mainSessions, OtherSessions: p.camera.otherSessions,
		VideoCodec: p.pipeline.videoCodec,
		CameraType: p.discovery.device.Type, CameraModel: p.discovery.device.Model,
		HardwareVersion: p.discovery.device.Hardware, FirmwareVersion: p.discovery.device.Firmware,
		DeviceInfo: p.discovery.deviceStatus, CapabilityInfo: p.discovery.capabilityStatus,
		Channels:        channels,
		VideoRecoveries: p.videoResyncs.Load(),
		LastError:       p.errClass,
	}
	if p.pipeline.audioMedia != nil {
		d.AudioCodec = p.pipeline.audioMedia.Codecs[0].Name
	}
	if p.history != nil {
		d.RefetchEnabled = true
		d.RefetchAttempts = p.history.attempts.Load()
		d.RefetchResumes = p.history.resumes.Load()
	}
	p.mu.Unlock()
	registry.mu.Unlock()
	if input != nil {
		stats := input.Stats()
		d.BoundaryAdjust = stats.BoundaryAdjustments
		d.VideoShort = stats.ShortVideoUnits
	}
	return d
}

func (p *Producer) MarshalJSON() ([]byte, error) {
	return json.Marshal(&producerDiagnostics{
		ID: p.ID, FormatName: p.FormatName, Protocol: p.Protocol, RemoteAddr: p.RemoteAddr,
		Medias: p.Medias, Recv: p.recvBytes.Load(), Send: p.send.Load(), Reolink: p.diagnostics(),
	})
}
