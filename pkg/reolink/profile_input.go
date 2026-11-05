package reolink

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
)

type profileInput interface {
	Read(context.Context) (baichuan.MediaPacket, error)
	ProbeTalk(context.Context, uint8) (baichuan.TalkFormat, error)
	StartTalk(context.Context, uint8) (talkSession, error)
	Discovery() discoverySnapshot
	Abort() error
	Close() error
}

type profileOpener func(context.Context, *Camera, source) (profileInput, error)

const capabilityProbeTimeout = 2 * time.Second

type discoverySnapshot struct {
	device           baichuan.DeviceInfo
	capabilities     baichuan.Capabilities
	deviceStatus     string
	capabilityStatus string
}

func (p *Profile) probeTalk() (baichuan.TalkFormat, bool, error) {
	p.talkMu.Lock()
	defer p.talkMu.Unlock()
	if p.talkKnown {
		return p.talkFormat, p.talkSupported, nil
	}
	p.mu.Lock()
	input := p.input
	state := p.state
	terminal := p.terminal
	p.mu.Unlock()
	if input == nil || state != profileActive {
		if terminal == nil {
			terminal = fmt.Errorf("reolink: profile %s", state)
		}
		return baichuan.TalkFormat{}, false, terminal
	}
	ctx, cancel := context.WithTimeout(p.ctx, p.sourceTimeout())
	format, err := input.ProbeTalk(ctx, p.key.Channel)
	cancel()
	if err != nil {
		var unsupported *baichuan.UnsupportedTalkError
		if !errors.As(err, &unsupported) {
			return baichuan.TalkFormat{}, false, fmt.Errorf("reolink: probe talkback: %w", err)
		}
		p.talkKnown = true
		return baichuan.TalkFormat{}, false, nil
	}
	p.talkKnown = true
	p.talkSupported = true
	p.talkFormat = format
	return format, true, nil
}

func (p *Profile) startTalk(ctx context.Context) (talkSession, error) {
	p.mu.Lock()
	input := p.input
	state := p.state
	terminal := p.terminal
	p.mu.Unlock()
	if input == nil || state != profileActive {
		if terminal == nil {
			terminal = fmt.Errorf("reolink: profile %s", state)
		}
		return nil, terminal
	}
	return input.StartTalk(ctx, p.key.Channel)
}

type baichuanInput struct {
	client    *baichuan.Client
	preview   *baichuan.Preview
	discovery discoverySnapshot
	once      sync.Once
	err       error
}

func openBaichuan(ctx context.Context, _ *Camera, s source) (profileInput, error) {
	client, err := baichuan.Dial(ctx, s.config)
	if err != nil {
		return nil, err
	}
	if err = client.Login(ctx); err != nil {
		return nil, errors.Join(err, client.Close())
	}
	discovery := discover(ctx, client, s.channel)
	preview, err := client.StartPreview(ctx, s.channel, s.stream)
	if err != nil {
		return nil, errors.Join(err, client.Close())
	}
	return &baichuanInput{client: client, preview: preview, discovery: discovery}, nil
}

func discover(ctx context.Context, client *baichuan.Client, channel uint8) discoverySnapshot {
	ctx, cancel := context.WithTimeout(ctx, capabilityProbeTimeout)
	defer cancel()
	var device baichuan.DeviceInfo
	var capabilities baichuan.Capabilities
	var deviceErr, capabilityErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		device, deviceErr = client.DeviceInfo(ctx)
	}()
	go func() {
		defer wg.Done()
		capabilities, capabilityErr = client.Capabilities(ctx, channel)
	}()
	wg.Wait()
	return discoverySnapshot{
		device: device, capabilities: capabilities,
		deviceStatus: discoveryStatus(deviceErr), capabilityStatus: discoveryStatus(capabilityErr),
	}
}

func discoveryStatus(err error) string {
	if err == nil {
		return "available"
	}
	return errorClass(err)
}

func (i *baichuanInput) Discovery() discoverySnapshot {
	return i.discovery
}

func (i *baichuanInput) Read(ctx context.Context) (baichuan.MediaPacket, error) {
	return i.preview.Read(ctx)
}

func (i *baichuanInput) ProbeTalk(ctx context.Context, channel uint8) (baichuan.TalkFormat, error) {
	return i.client.ProbeTalk(ctx, channel)
}

func (i *baichuanInput) StartTalk(ctx context.Context, channel uint8) (talkSession, error) {
	return i.client.StartTalk(ctx, channel)
}

func (i *baichuanInput) Close() error {
	return i.close(true)
}

func (i *baichuanInput) Abort() error {
	return i.close(false)
}

func (i *baichuanInput) close(graceful bool) error {
	i.once.Do(func() {
		if graceful {
			i.err = errors.Join(i.preview.Close(), i.client.Close())
		} else {
			i.err = errors.Join(i.client.Close(), i.preview.Close())
		}
	})
	return i.err
}
