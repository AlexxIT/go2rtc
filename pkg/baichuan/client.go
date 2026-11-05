package baichuan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

var ErrPreviewOverflow = errors.New("baichuan: preview queue overflow")

type pendingKey struct {
	command  uint32
	sequence uint16
}

type pendingRequest struct {
	response chan message
	login    bool
}

type previewKey struct {
	channel uint8
	stream  uint8
}

type Client struct {
	cfg  Config
	conn transport

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	wg     sync.WaitGroup

	closeOnce sync.Once
	errMu     sync.Mutex
	err       error
	closeErr  error

	cipherMu sync.RWMutex
	cipher   cipherState

	sendMu sync.Mutex
	reqMu  sync.Mutex
	seq    uint16
	reqs   map[pendingKey]pendingRequest

	previewMu    sync.RWMutex
	previews     map[previewKey]*Preview
	mediaPackets atomic.Uint64

	loginMu sync.Mutex
	logged  bool

	capabilityMu sync.Mutex
	capabilities map[uint8]Capabilities
	deviceMu     sync.Mutex
	device       DeviceInfo
	deviceKnown  bool
}

func (c *Client) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprint(state, "baichuan.Client")
}

func Dial(ctx context.Context, cfg Config) (*Client, error) {
	cfg, err := cfg.normalized()
	if err != nil {
		return nil, err
	}
	var conn transport
	if cfg.uid != "" {
		var discovery *uidDiscovery
		discovery, err = discoverUID(ctx, cfg.uid, cfg.UIDLocalAddr, cfg.UIDBroadcastAddr, cfg.Timeout)
		if err == nil {
			conn, err = newUIDConn(discovery, cfg.Timeout)
		}
	} else {
		conn, err = dialTCP(ctx, cfg)
	}
	if err != nil {
		return nil, fmt.Errorf("baichuan: dial camera: %w", err)
	}
	return newClient(ctx, cfg, conn), nil
}

func newClient(ctx context.Context, cfg Config, conn transport) *Client {
	lifetime, cancel := context.WithCancel(context.WithoutCancel(ctx))
	c := &Client{
		cfg: cfg, conn: conn, ctx: lifetime, cancel: cancel, done: make(chan struct{}),
		reqs: make(map[pendingKey]pendingRequest), previews: make(map[previewKey]*Preview),
	}
	c.wg.Add(1)
	go c.readLoop()
	return c
}

func (c *Client) Done() <-chan struct{} {
	return c.done
}

func (c *Client) Err() error {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	return c.err
}

func (c *Client) Close() error {
	c.shutdown(context.Canceled)
	// Login may add keepalive work; wait for it before waiting on the group.
	c.loginMu.Lock()
	c.loginMu.Unlock()
	c.wg.Wait()
	c.errMu.Lock()
	err := c.closeErr
	c.errMu.Unlock()
	return err
}

func (c *Client) shutdown(err error) {
	c.closeOnce.Do(func() {
		c.errMu.Lock()
		c.err = err
		c.errMu.Unlock()
		c.cancel()
		close(c.done)
		closeErr := c.conn.Close()
		c.errMu.Lock()
		c.closeErr = closeErr
		c.errMu.Unlock()
	})
}

func (c *Client) readLoop() {
	defer c.wg.Done()
	for {
		value, err := readFrame(c.conn, c.cfg.Limits)
		if err != nil {
			if !errors.Is(err, io.EOF) || c.ctx.Err() == nil {
				c.shutdown(fmt.Errorf("baichuan: read message: %w", err))
			}
			return
		}

		key := pendingKey{command: value.header.Command, sequence: value.header.Sequence}
		pending := c.pending(key)
		if key.command == commandLogin && pending.response == nil {
			continue
		}
		state := c.cipherState(value.header, pending)
		preview := c.getPreview(value.header)
		binaryHint := value.header.Command == commandPreview && pending.response == nil && preview != nil
		msg, err := decodeFrame(value, state, binaryHint)
		if err != nil {
			c.shutdown(fmt.Errorf("baichuan: decode message: %w", err))
			return
		}
		if msg.binary {
			if msg.header.Command == commandPreview && preview != nil {
				preview.deliver(msg)
			}
			continue
		}
		if pending.response != nil {
			if err = c.complete(key, pending, msg, state); err != nil {
				c.shutdown(err)
				return
			}
		}
	}
}

func (c *Client) cipherState(h header, pending pendingRequest) cipherState {
	c.cipherMu.RLock()
	state := c.cipher
	c.cipherMu.RUnlock()
	if h.Command != commandLogin || pending.response == nil {
		return state
	}
	if !pending.login {
		state = cipherState{}
	}
	if mode, ok := negotiatedEncryption(h.ResponseCode); ok {
		state.mode = mode
		state.sessionMode = mode
		if h.ResponseCode == 0xdd12 {
			// Authentication remains BC; subsequent control uses AES.
			state.sessionMode = encryptionAES
		}
	}
	return state
}

func (c *Client) pending(key pendingKey) pendingRequest {
	c.reqMu.Lock()
	defer c.reqMu.Unlock()
	return c.reqs[key]
}

func (c *Client) roundTrip(ctx context.Context, req request) (message, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	key, pending, err := c.reserve(req)
	if err != nil {
		return message{}, err
	}
	defer c.release(key, pending)
	ch := pending.response
	req.sequence = key.sequence

	c.cipherMu.RLock()
	state := c.cipher
	c.cipherMu.RUnlock()
	packet, err := encodeRequest(req, c.cfg.Limits, state)
	if err != nil {
		return message{}, err
	}
	c.sendMu.Lock()
	if err = ctx.Err(); err != nil {
		c.sendMu.Unlock()
		return message{}, err
	}
	err = writeFull(ctx, c.conn, c.cfg.Timeout, packet)
	c.sendMu.Unlock()
	if err != nil {
		err = fmt.Errorf("baichuan: write command %d: %w", req.command, err)
		c.shutdown(err)
		return message{}, err
	}

	select {
	case msg := <-ch:
		return checkedResponse(msg)
	case <-ctx.Done():
		select {
		case msg := <-ch:
			return checkedResponse(msg)
		default:
			return message{}, ctx.Err()
		}
	case <-c.done:
		select {
		case msg := <-ch:
			return checkedResponse(msg)
		default:
			return message{}, c.Err()
		}
	}
}

func checkedResponse(msg message) (message, error) {
	if err := responseError(msg.header); err != nil {
		return message{}, err
	}
	return msg, nil
}

func (c *Client) writeRequest(ctx context.Context, req request) error {
	req.sequence = c.nextSequence(req.command)
	c.cipherMu.RLock()
	state := c.cipher
	c.cipherMu.RUnlock()
	packet, err := encodeRequest(req, c.cfg.Limits, state)
	if err != nil {
		return err
	}
	c.sendMu.Lock()
	if err = ctx.Err(); err != nil {
		c.sendMu.Unlock()
		return err
	}
	err = writeFull(ctx, c.conn, c.cfg.Timeout, packet)
	c.sendMu.Unlock()
	if err != nil {
		err = fmt.Errorf("baichuan: write command %d: %w", req.command, err)
		c.shutdown(err)
	}
	return err
}

func (c *Client) nextSequence(command uint32) uint16 {
	c.reqMu.Lock()
	defer c.reqMu.Unlock()
	for {
		sequence := c.seq
		c.seq++
		if _, ok := c.reqs[pendingKey{command: command, sequence: sequence}]; !ok {
			return sequence
		}
	}
}

func (c *Client) reserve(req request) (pendingKey, pendingRequest, error) {
	c.reqMu.Lock()
	defer c.reqMu.Unlock()
	if len(c.reqs) >= c.cfg.Limits.MaxPending {
		return pendingKey{}, pendingRequest{}, fmt.Errorf("baichuan: pending request limit reached")
	}
	for i := 0; i < 1<<16; i++ {
		key := pendingKey{command: req.command, sequence: c.seq}
		c.seq++
		if _, ok := c.reqs[key]; !ok {
			pending := pendingRequest{
				response: make(chan message, 1), login: req.command == commandLogin && req.class != classLegacy,
			}
			c.reqs[key] = pending
			return key, pending, nil
		}
	}
	return pendingKey{}, pendingRequest{}, fmt.Errorf("baichuan: no request sequence available")
}

func (c *Client) release(key pendingKey, pending pendingRequest) {
	c.reqMu.Lock()
	if c.reqs[key].response == pending.response {
		delete(c.reqs, key)
	}
	c.reqMu.Unlock()
}

func (c *Client) complete(key pendingKey, pending pendingRequest, msg message, state cipherState) error {
	c.reqMu.Lock()
	defer c.reqMu.Unlock()
	if c.reqs[key].response != pending.response {
		return nil
	}
	delete(c.reqs, key)
	if key.command == commandLogin && responseError(msg.header) == nil {
		if pending.login {
			state.mode = state.sessionMode
		}
		c.cipherMu.Lock()
		c.cipher = state
		c.cipherMu.Unlock()
	}
	select {
	case pending.response <- msg:
		return nil
	default:
		return fmt.Errorf("baichuan: duplicate response for command %d sequence %d", key.command, key.sequence)
	}
}

func (c *Client) Login(ctx context.Context) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	if c.logged {
		return nil
	}

	nonceMessage, err := c.roundTrip(ctx, request{command: commandLogin, class: classLegacy, forceBC: true})
	if err != nil {
		return fmt.Errorf("baichuan: request login nonce: %w", err)
	}
	nonce, err := parseNonce(nonceMessage.payload)
	if err != nil {
		return fmt.Errorf("baichuan: parse login nonce: %w", err)
	}
	c.cipherMu.Lock()
	c.cipher.setAESKey(deriveAESKey(nonce, c.cfg.password))
	c.cipherMu.Unlock()

	body, err := buildLogin(c.cfg.username, c.cfg.password, nonce)
	if err != nil {
		return fmt.Errorf("baichuan: build login: %w", err)
	}
	mode, _ := negotiatedEncryption(nonceMessage.header.ResponseCode)
	if _, err = c.roundTrip(ctx, request{
		command: commandLogin, class: classOffset, payload: body, forceBC: mode == encryptionAES,
	}); err != nil {
		return fmt.Errorf("baichuan: login: %w", err)
	}
	c.logged = true
	// Active UID sessions maintain liveness with transport data and ACKs. The
	// request/response ping is TCP-only and can fill a UID send window on
	// firmware that doesn't answer it.
	if _, ok := c.conn.(*uidConn); ok {
		return nil
	}
	c.wg.Add(1)
	go c.keepAlive()
	return nil
}

func (c *Client) keepAlive() {
	defer c.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	failures := 0
	lastPackets := c.mediaPackets.Load()
	for {
		select {
		case <-ticker.C:
			if c.keepAliveStep(&failures, &lastPackets) {
				return
			}
		case <-c.done:
			return
		}
	}
}

func (c *Client) keepAliveStep(failures *int, lastPackets *uint64) bool {
	ctx, cancel := context.WithTimeout(c.ctx, 4*time.Second)
	_, err := c.roundTrip(ctx, request{command: commandPing, class: classOffset})
	cancel()
	packets := c.mediaPackets.Load()
	if err == nil {
		*failures = 0
	} else if errors.Is(err, context.DeadlineExceeded) && packets != *lastPackets {
		*failures = 0
	} else {
		*failures++
	}
	*lastPackets = packets
	if *failures == 3 {
		c.shutdown(fmt.Errorf("baichuan: keepalive failed: %w", err))
		return true
	}
	return false
}
