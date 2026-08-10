package baichuan

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

const previewQueueMessages = 64

func (c *Client) StartPreview(ctx context.Context, channel uint8, stream Stream) (*Preview, error) {
	if err := c.Login(ctx); err != nil {
		return nil, err
	}
	streamType, handle, err := stream.params()
	if err != nil {
		return nil, err
	}
	body, err := buildPreview(channel, stream, handle)
	if err != nil {
		return nil, fmt.Errorf("baichuan: build preview request: %w", err)
	}
	parser, err := NewMediaParser(c.cfg.Limits)
	if err != nil {
		return nil, err
	}
	p := &Preview{
		client: c, key: previewKey{channel: channel, stream: streamType}, stream: stream,
		handle: handle, messages: make(chan message, previewQueueMessages),
		queueLimit: int64(c.cfg.Limits.MaxMediaBuffer), done: make(chan struct{}), parser: parser,
	}
	if err = c.addPreview(p); err != nil {
		return nil, err
	}
	if _, err = c.roundTrip(ctx, request{
		command: commandPreview, channel: channel, stream: streamType, class: classOffset, payload: body,
	}); err != nil {
		p.closeLocal(err)
		return nil, fmt.Errorf("baichuan: start preview: %w", err)
	}
	return p, nil
}

func (c *Client) addPreview(p *Preview) error {
	c.previewMu.Lock()
	defer c.previewMu.Unlock()
	if _, ok := c.previews[p.key]; ok {
		return fmt.Errorf("baichuan: preview already active for channel %d stream %d", p.key.channel, p.key.stream)
	}
	c.previews[p.key] = p
	return nil
}

func (c *Client) removePreview(p *Preview) {
	c.previewMu.Lock()
	if c.previews[p.key] == p {
		delete(c.previews, p.key)
	}
	c.previewMu.Unlock()
}

func (c *Client) getPreview(h header) *Preview {
	c.previewMu.RLock()
	p := c.previews[previewKey{channel: h.Channel, stream: h.Stream}]
	c.previewMu.RUnlock()
	return p
}

type Preview struct {
	client *Client
	key    previewKey
	stream Stream
	handle uint32

	messages   chan message
	queueLimit int64
	queueBytes atomic.Int64
	done       chan struct{}
	close      sync.Once
	stop       sync.Once
	stopErr    error
	errMu      sync.Mutex
	err        error

	readMu  sync.Mutex
	parser  *MediaParser
	packets []MediaPacket
	next    int
}

func (p *Preview) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "baichuan.Preview{Channel:%d, Stream:%q}", p.key.channel, p.stream)
}

func (p *Preview) deliver(msg message) {
	select {
	case <-p.done:
		return
	default:
	}
	size := int64(len(msg.extension) + len(msg.payload))
	if !p.reserveQueue(size) {
		p.closeLocal(ErrPreviewOverflow)
		return
	}
	select {
	case p.messages <- msg:
	case <-p.done:
		p.queueBytes.Add(-size)
	default:
		p.queueBytes.Add(-size)
		p.closeLocal(ErrPreviewOverflow)
	}
}

func (p *Preview) reserveQueue(size int64) bool {
	for {
		queued := p.queueBytes.Load()
		if size > p.queueLimit-queued {
			return false
		}
		if !p.queueBytes.CompareAndSwap(queued, queued+size) {
			continue
		}
		return true
	}
}

func (p *Preview) Read(ctx context.Context) (MediaPacket, error) {
	p.readMu.Lock()
	defer p.readMu.Unlock()
	for {
		select {
		case <-p.done:
			return MediaPacket{}, p.Err()
		case <-p.client.done:
			return MediaPacket{}, p.client.Err()
		default:
		}
		if p.next < len(p.packets) {
			packet := p.packets[p.next]
			p.packets[p.next] = MediaPacket{}
			p.next++
			return packet, nil
		}
		select {
		case msg := <-p.messages:
			p.queueBytes.Add(-int64(len(msg.extension) + len(msg.payload)))
			select {
			case <-p.done:
				return MediaPacket{}, p.Err()
			default:
			}
			residue := len(p.parser.buf)
			packets, err := p.parser.appendOwnedTo(msg.payload, p.packets)
			if err != nil {
				err = fmt.Errorf("baichuan: parse media with residue %d, extension %d, encrypted prefix %d (present %t), check position %d (present %t), payload %d, parsed %d, remaining %d: %w",
					residue, len(msg.extension), msg.encrypt, msg.hasEncryptLen, msg.checkPos, msg.hasCheckPos,
					len(msg.payload), len(packets), len(p.parser.buf), err)
				p.closeLocal(err)
				return MediaPacket{}, err
			}
			p.packets = packets
			p.next = 0
		case <-p.done:
			return MediaPacket{}, p.Err()
		case <-p.client.done:
			return MediaPacket{}, p.client.Err()
		case <-ctx.Done():
			return MediaPacket{}, ctx.Err()
		}
	}
}

func (p *Preview) Err() error {
	p.errMu.Lock()
	defer p.errMu.Unlock()
	if p.err == nil {
		return io.EOF
	}
	return p.err
}

func (p *Preview) closeLocal(err error) {
	p.close.Do(func() {
		p.errMu.Lock()
		p.err = err
		p.errMu.Unlock()
		p.client.removePreview(p)
		close(p.done)
	})
}

func (p *Preview) Close() error {
	p.stop.Do(func() {
		p.closeLocal(context.Canceled)
		if p.client.ctx.Err() != nil {
			return
		}
		body, err := buildStopPreview(p.key.channel, p.handle)
		if err != nil {
			p.stopErr = err
			return
		}
		ctx, cancel := context.WithTimeout(p.client.ctx, p.client.cfg.Timeout)
		defer cancel()
		_, err = p.client.roundTrip(ctx, request{
			command: commandStopPreview, channel: p.key.channel, stream: p.key.stream,
			class: classOffset, payload: body,
		})
		if err != nil {
			p.stopErr = fmt.Errorf("baichuan: stop preview: %w", err)
		}
	})
	return p.stopErr
}
