package baichuan

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
)

type UnsupportedTalkError struct {
	Reason string
}

func (e *UnsupportedTalkError) Error() string {
	return "baichuan: talkback unsupported: " + e.Reason
}

type TalkFormat struct {
	SampleRate      uint32
	SamplePrecision uint32
	SamplesPerBlock uint32
}

func (f TalkFormat) BytesPerBlock() int {
	return 4 + int(f.SamplesPerBlock)/2
}

func (c *Client) ProbeTalk(ctx context.Context, channel uint8) (TalkFormat, error) {
	if err := c.Login(ctx); err != nil {
		return TalkFormat{}, err
	}
	config, err := c.talkConfig(ctx, channel)
	if err != nil {
		return TalkFormat{}, err
	}
	return formatOf(config), nil
}

func (c *Client) talkConfig(ctx context.Context, channel uint8) (talkConfig, error) {
	extension, err := buildTalkExtension(channel, false)
	if err != nil {
		return talkConfig{}, err
	}
	msg, err := c.roundTrip(ctx, request{
		command: commandTalkAbility, channel: channel, class: classOffset, extension: extension,
	})
	if err != nil {
		var status *StatusError
		if errors.As(err, &status) && unsupportedTalkStatus(status.Code) {
			return talkConfig{}, &UnsupportedTalkError{Reason: fmt.Sprintf("camera status %d", status.Code)}
		}
		return talkConfig{}, fmt.Errorf("baichuan: query talk ability: %w", err)
	}
	ability, err := decodeTalkAbility(msg.payload)
	if err != nil {
		return talkConfig{}, fmt.Errorf("baichuan: decode talk ability: %w", err)
	}
	return selectTalkConfig(channel, ability)
}

func unsupportedTalkStatus(code uint16) bool {
	switch code {
	case 400, 404, 405, 501:
		return true
	default:
		return false
	}
}

func formatOf(config talkConfig) TalkFormat {
	return TalkFormat{
		SampleRate: config.Audio.SampleRate, SamplePrecision: config.Audio.SamplePrecision,
		SamplesPerBlock: config.Audio.SamplesPerBlock,
	}
}

type Talk struct {
	client    *Client
	channel   uint8
	format    TalkFormat
	extension []byte

	mu        sync.Mutex
	closed    bool
	sequence  uint16
	closeOnce sync.Once
	closeDone chan struct{}
	closeErr  error
}

func (t *Talk) String() string {
	return fmt.Sprintf("baichuan.Talk{Channel:%d, Audio:%+v}", t.channel, t.format)
}

func (t *Talk) GoString() string {
	return t.String()
}

func (c *Client) StartTalk(ctx context.Context, channel uint8) (*Talk, error) {
	if err := c.Login(ctx); err != nil {
		return nil, err
	}
	config, err := c.talkConfig(ctx, channel)
	if err != nil {
		return nil, err
	}
	if err = c.startTalk(ctx, config); err != nil {
		return nil, err
	}
	extension, err := buildTalkExtension(channel, true)
	if err != nil {
		return nil, errors.Join(err, c.stopTalk(ctx, channel))
	}
	return &Talk{
		client: c, channel: channel, format: formatOf(config), extension: extension, closeDone: make(chan struct{}),
	}, nil
}

func (c *Client) startTalk(ctx context.Context, config talkConfig) error {
	extension, err := buildTalkExtension(config.Channel, false)
	if err != nil {
		return err
	}
	body, err := marshalDocument(talkConfigEnvelope{Config: config})
	if err != nil {
		return err
	}
	req := request{
		command: commandTalkConfig, channel: config.Channel, class: classOffset,
		extension: extension, payload: body,
	}
	if _, err = c.roundTrip(ctx, req); err != nil {
		var status *StatusError
		if !errors.As(err, &status) || status.Code != 422 {
			return fmt.Errorf("baichuan: configure talk: %w", err)
		}
		if resetErr := c.stopTalk(ctx, config.Channel); resetErr != nil {
			return errors.Join(fmt.Errorf("baichuan: reset stale talk: %w", err), resetErr)
		}
		if _, err = c.roundTrip(ctx, req); err != nil {
			return fmt.Errorf("baichuan: configure talk after reset: %w", err)
		}
	}
	return nil
}

func (c *Client) stopTalk(ctx context.Context, channel uint8) error {
	extension, err := buildTalkExtension(channel, false)
	if err != nil {
		return err
	}
	_, err = c.roundTrip(ctx, request{
		command: commandStopTalk, channel: channel, class: classOffset, extension: extension,
	})
	var status *StatusError
	if errors.As(err, &status) && status.Code == 422 {
		return nil
	}
	return err
}

func (t *Talk) Format() TalkFormat {
	return t.format
}

func (t *Talk) WriteBlock(ctx context.Context, block []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return fmt.Errorf("baichuan: write talk: %w", context.Canceled)
	}
	if len(block) != t.format.BytesPerBlock() {
		return fmt.Errorf("baichuan: ADPCM block size %d, want %d", len(block), t.format.BytesPerBlock())
	}
	payload, err := buildTalkPayload(block, t.sequence)
	if err != nil {
		return err
	}
	t.sequence++
	return t.client.writeRequest(ctx, request{
		command: commandTalkData, channel: t.channel, class: classOffset,
		extension: t.extension, payload: payload, binary: true,
	})
}

func (t *Talk) Close(ctx context.Context) error {
	t.closeOnce.Do(func() {
		t.mu.Lock()
		t.closed = true
		t.mu.Unlock()
		go func() {
			if t.client.ctx.Err() == nil {
				closeCtx, cancel := context.WithTimeout(t.client.ctx, t.client.cfg.Timeout)
				t.closeErr = t.client.stopTalk(closeCtx, t.channel)
				cancel()
			}
			close(t.closeDone)
		}()
	})
	select {
	case <-t.closeDone:
		return t.closeErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func buildTalkPayload(block []byte, sequence uint16) ([]byte, error) {
	if len(block) > int(^uint16(0))-4 {
		return nil, fmt.Errorf("baichuan: ADPCM block too large: %d", len(block))
	}
	size := len(block) + 4
	total := 8 + size + int(padding(uint32(size)))
	b := make([]byte, total)
	binary.LittleEndian.PutUint32(b, mediaADPCM)
	binary.LittleEndian.PutUint16(b[4:6], uint16(size))
	binary.LittleEndian.PutUint16(b[6:8], uint16(size))
	binary.LittleEndian.PutUint16(b[8:10], 0x0100)
	binary.LittleEndian.PutUint16(b[10:12], sequence)
	copy(b[12:], block)
	return b, nil
}
