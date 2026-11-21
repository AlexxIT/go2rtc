package miss

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/crypto"
)

type testConn struct {
	Conn
	key       []byte
	mu        sync.Mutex
	commands  []uint32
	cmdBodies []string
	onCommand func(cmd uint32, body string)
	audioOut  int
	closes    int
	packets   chan testPacket
	closed    chan struct{}
	closeOnce sync.Once
}

type testPacket struct {
	hdr     []byte
	payload []byte
}

func newTestConn(key []byte) *testConn {
	return &testConn{
		key:     key,
		packets: make(chan testPacket, 32),
		closed:  make(chan struct{}),
	}
}

func (c *testConn) Protocol() string            { return "cs2+udp" }
func (c *testConn) Version() string             { return "test" }
func (c *testConn) RemoteAddr() net.Addr        { return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1234} }
func (c *testConn) SetDeadline(time.Time) error { return nil }

func (c *testConn) WriteCommand(cmd uint32, data []byte) error {
	if cmd != cmdEncoded {
		return errors.New("unexpected unencrypted command")
	}
	plain, err := crypto.Decode(data, c.key)
	if err != nil {
		return err
	}
	code := binary.BigEndian.Uint32(plain)
	body := string(plain[4:])
	c.mu.Lock()
	c.commands = append(c.commands, code)
	c.cmdBodies = append(c.cmdBodies, body)
	cb := c.onCommand
	c.mu.Unlock()
	if cb != nil {
		cb(code, body)
	}
	return nil
}

func (c *testConn) WritePacket(_, _ []byte) error {
	c.mu.Lock()
	c.audioOut++
	c.mu.Unlock()
	return nil
}

func (c *testConn) ReadPacket() ([]byte, []byte, error) {
	select {
	case pkt := <-c.packets:
		return pkt.hdr, pkt.payload, nil
	case <-c.closed:
		return nil, nil, io.EOF
	}
}

func (c *testConn) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closes++
		c.mu.Unlock()
		close(c.closed)
	})
	return nil
}

func (c *testConn) pushVideo(codec, seq, flags uint32, ch uint8, tagged bool) {
	raw := []byte{0x00, 0x00, 0x00, 0x01, 0x40, 0x01, 0x0c, 0x01}
	if codec == codecH264 {
		raw = []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x1e}
	}
	enc, _ := crypto.Encode(raw, c.key)
	hdr := make([]byte, hdrSize)
	binary.LittleEndian.PutUint32(hdr[4:], codec)
	binary.LittleEndian.PutUint32(hdr[8:], seq)
	binary.LittleEndian.PutUint32(hdr[12:], flags)
	if tagged {
		binary.LittleEndian.PutUint32(hdr[24:], 1)
		hdr[28] = ch
	} else {
		binary.LittleEndian.PutUint32(hdr[24:], 0xffff)
		hdr[28] = 0xff
	}
	c.packets <- testPacket{hdr: hdr, payload: enc}
}

func (c *testConn) pushAudio(codec, seq uint32) {
	enc, _ := crypto.Encode([]byte{0xd5, 0xd5, 0xd5, 0xd5}, c.key)
	hdr := make([]byte, hdrSize)
	binary.LittleEndian.PutUint32(hdr[4:], codec)
	binary.LittleEndian.PutUint32(hdr[8:], seq)
	c.packets <- testPacket{hdr: hdr, payload: enc}
}

func newTestProducer(t *testing.T, client *Client, audio bool) *Producer {
	t.Helper()
	medias, err := probe(client, audio)
	if err != nil {
		t.Fatal(err)
	}
	return &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "xiaomi/miss",
			Protocol:   client.Protocol(),
			RemoteAddr: client.RemoteAddr().String(),
			UserAgent:  client.Version(),
			Medias:     medias,
			Transport:  client,
		},
		client: client,
	}
}

func TestParseChannel(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want uint8
	}{{"", 0}, {"0", 0}, {"1", 1}, {"2", 1}} {
		if got, err := parseChannel(tc.in); err != nil || got != tc.want {
			t.Fatalf("parseChannel(%q) = %d, %v", tc.in, got, err)
		}
	}
	if _, err := Dial("xiaomi://127.0.0.1?did=1&channel=invalid"); err == nil {
		t.Fatal("expected invalid channel error")
	}
}

func TestMuxChannelIsolationAndAudio(t *testing.T) {
	m := &mux{
		client:       &Client{},
		done:         make(chan struct{}),
		lensTagReady: make(chan struct{}),
		startedMask:  1,
		quality:      [2]string{"2", "1"},
	}
	c0, err := m.attach(0, "hd", true)
	if err != nil {
		t.Fatal(err)
	}

	// Untagged single-channel video is accepted, other lens is rejected.
	m.dispatch(&Packet{CodecID: codecH265, Payload: []byte{0, 0, 0, 1, 0x40}})
	m.dispatch(&Packet{CodecID: codecH265, Channel: 1, ChannelOK: true, Payload: []byte{0, 0, 0, 1, 0x40}})
	if len(c0.sub.packets) != 1 {
		t.Fatalf("expected 1 video packet on ch0, got %d", len(c0.sub.packets))
	}

	// Dual-channel separates lenses, drops untagged video, and respects audio flag.
	m.startedMask = 3
	c1, err := m.attach(1, "sd", false)
	if err != nil {
		t.Fatal(err)
	}
	m.dispatch(&Packet{CodecID: codecH265, Channel: 1, ChannelOK: true, Payload: []byte{0, 0, 0, 1, 0x40}})
	m.dispatch(&Packet{CodecID: codecH265, Payload: []byte{0, 0, 0, 1, 0x40}})
	m.dispatch(&Packet{CodecID: codecPCMA, Payload: []byte{1, 2, 3}})
	if len(c0.sub.packets) != 2 || len(c1.sub.packets) != 1 {
		t.Fatalf("routing mismatch: ch0=%d ch1=%d", len(c0.sub.packets), len(c1.sub.packets))
	}
}

func TestMuxCW500LensFlags(t *testing.T) {
	key := make([]byte, 32)
	conn := newTestConn(key)
	conn.pushVideo(codecH265, 1, 0x014EA000, 0, true)
	c := &Client{Conn: conn, key: key, model: "isa.camera.hlmax"}
	pkt, err := c.ReadPacket()
	if err != nil || pkt.ChannelOK || pkt.Flags != 0x014EA000 {
		t.Fatalf("CW500 must use Flags, got err=%v chOK=%v flags=%x", err, pkt.ChannelOK, pkt.Flags)
	}

	m := &mux{
		client:       c,
		done:         make(chan struct{}),
		lensTagReady: make(chan struct{}),
		startedMask:  1,
		quality:      [2]string{"2", "1"},
	}
	c0, err := m.attach(0, "hd", false)
	if err != nil {
		t.Fatal(err)
	}
	c1, err := m.attach(1, "sd", false)
	if err != nil {
		t.Fatal(err)
	}

	// Learn lens 0 tag while only lens 0 has started, then verify HD/SD routing.
	m.dispatch(&Packet{CodecID: codecH265, Flags: 0x000EA000, Payload: []byte{0, 0, 0, 1, 0x40}})
	m.startedMask = 3
	m.dispatch(&Packet{CodecID: codecH265, Flags: 0x0146A000, Payload: []byte{0, 0, 0, 1, 0x40}})
	m.dispatch(&Packet{CodecID: codecH265, Flags: 0x004EA000, Payload: []byte{0, 0, 0, 1, 0x40}}) // invalid tag
	if len(c0.sub.packets) != 1 || len(c1.sub.packets) != 1 {
		t.Fatalf("CW500 routing mismatch: ch0=%d ch1=%d", len(c0.sub.packets), len(c1.sub.packets))
	}
}

func TestMuxDualChannelProducerLifecycle(t *testing.T) {
	key := make([]byte, 32)
	conn := newTestConn(key)
	conn.onCommand = func(cmd uint32, body string) {
		if cmd != cmdVideoStart {
			return
		}
		switch body {
		case `{"videoquality":2,"enableaudio":1}`:
			conn.pushVideo(codecH265, 1, 0x000EA000, 0, false)
			conn.pushAudio(codecPCMA, 1)
		case `{"videoquality":2,"videoquality2":1,"enableaudio":1}`:
			conn.pushVideo(codecH265, 2, 0x014EA000, 1, false)
		}
	}

	m := newMux(&Client{Conn: conn, key: key, model: "isa.camera.hlmax"})

	c0, err := m.attach(0, "hd", true)
	if err != nil {
		t.Fatal(err)
	}
	if err = c0.StartMedia("0", "hd", "1"); err != nil {
		t.Fatal(err)
	}
	p0 := newTestProducer(t, c0, true)
	done0 := make(chan error, 1)
	go func() { done0 <- p0.Start() }()

	// Channel 1 joins with audio=0: session keeps audio enabled, ch1 exposes video only.
	c1, err := m.attach(1, "sd", false)
	if err != nil {
		t.Fatal(err)
	}
	if err = c1.StartMedia("2", "sd", "0"); err != nil {
		t.Fatal(err)
	}
	p1 := newTestProducer(t, c1, false)
	if len(p1.Medias) != 1 || p1.Medias[0].Kind != core.KindVideo {
		t.Fatalf("expected video-only medias for audio=0, got %v", p1.Medias)
	}
	done1 := make(chan error, 1)
	go func() { done1 <- p1.Start() }()

	// Concurrent backchannel write and command must not race.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = c0.WriteAudio(codecPCMA, []byte{1, 2, 3, 4})
	}()
	go func() {
		defer wg.Done()
		_ = c1.StartSpeaker()
	}()
	wg.Wait()

	conn.mu.Lock()
	cmds := append([]string(nil), conn.cmdBodies...)
	conn.mu.Unlock()
	if len(cmds) < 2 || cmds[1] != `{"videoquality":2,"videoquality2":1,"enableaudio":1}` {
		t.Fatalf("unexpected start commands: %v", cmds)
	}

	if err = p1.Stop(); err != nil {
		t.Fatal(err)
	}
	if err = <-done1; !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF after p1 stop, got %v", err)
	}
	if err = c1.StartMedia("2", "sd", "0"); !errors.Is(err, io.EOF) {
		t.Fatalf("closed client StartMedia must return io.EOF, got %v", err)
	}
	conn.mu.Lock()
	cmdCount, audioCount := len(conn.commands), conn.audioOut
	conn.mu.Unlock()
	if err = c1.StartSpeaker(); !errors.Is(err, io.EOF) {
		t.Fatalf("closed client StartSpeaker must return io.EOF, got %v", err)
	}
	if err = c1.WriteAudio(codecPCMA, []byte{1, 2, 3, 4}); !errors.Is(err, io.EOF) {
		t.Fatalf("closed client WriteAudio must return io.EOF, got %v", err)
	}
	conn.mu.Lock()
	if len(conn.commands) != cmdCount || conn.audioOut != audioCount {
		conn.mu.Unlock()
		t.Fatal("closed client wrote to shared connection")
	}
	conn.mu.Unlock()
	conn.mu.Lock()
	closes := conn.closes
	conn.mu.Unlock()
	if closes != 0 {
		t.Fatal("closing one channel must keep shared client open")
	}

	if err = p0.Stop(); err != nil {
		t.Fatal(err)
	}
	if err = <-done0; !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF after p0 stop, got %v", err)
	}
	conn.mu.Lock()
	closes = conn.closes
	lastCmd := conn.commands[len(conn.commands)-1]
	conn.mu.Unlock()
	if closes != 1 || lastCmd != cmdVideoStop {
		t.Fatalf("expected StopMedia + single close on final stop, got closes=%d cmd=%x", closes, lastCmd)
	}
}

func TestMuxRejectsConflictingQuality(t *testing.T) {
	key := make([]byte, 32)
	conn := newTestConn(key)
	conn.onCommand = func(cmd uint32, body string) {
		if cmd != cmdVideoStart {
			return
		}
		switch body {
		case `{"videoquality":2,"enableaudio":0}`:
			conn.pushVideo(codecH265, 1, 0, 0, true)
		case `{"videoquality":2,"videoquality2":1,"enableaudio":0}`:
			conn.pushVideo(codecH265, 2, 0, 1, true)
		}
	}

	m := newMux(&Client{Conn: conn, key: key, model: "chuangmi.camera.test"})

	c0, err := m.attach(0, "hd", false)
	if err != nil {
		t.Fatal(err)
	}
	if err = c0.StartMedia("0", "hd", "0"); err != nil {
		t.Fatal(err)
	}

	// Active lens requesting a different quality must return errMuxIncompatible.
	if _, err = m.attach(0, "sd", false); !errors.Is(err, errMuxIncompatible) {
		t.Fatalf("expected errMuxIncompatible, got %v", err)
	}

	// Same normalized quality ("" == "hd") is allowed.
	c0Dup, err := m.attach(0, "", false)
	if err != nil {
		t.Fatalf("same normalized quality should share mux: %v", err)
	}
	_ = c0Dup.Close()

	// Different lens with SD shares the session cleanly.
	c1, err := m.attach(1, "sd", false)
	if err != nil {
		t.Fatal(err)
	}
	if err = c1.StartMedia("2", "sd", "0"); err != nil {
		t.Fatal(err)
	}

	// Even after ch0 stops while ch1 remains active, changing ch0 quality must
	// still return errMuxIncompatible instead of probing stale SPS/VPS.
	_ = c0.Close()
	if _, err = m.attach(0, "sd", false); !errors.Is(err, errMuxIncompatible) {
		t.Fatalf("expected errMuxIncompatible after ch0 stop with different quality, got %v", err)
	}

	_ = c1.Close()
}

func TestMuxCloseDoesNotRaceConcurrentAttach(t *testing.T) {
	key := make([]byte, 32)
	for range 200 {
		conn := newTestConn(key)
		m := newMux(&Client{Conn: conn, key: key, model: "chuangmi.camera.test"})
		c0, err := m.attach(0, "hd", false)
		if err != nil {
			t.Fatal(err)
		}

		start := make(chan struct{})
		var c1 *Client
		var attachErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_ = c0.Close()
		}()
		go func() {
			defer wg.Done()
			<-start
			c1, attachErr = m.attach(1, "sd", false)
		}()
		close(start)
		wg.Wait()

		if attachErr == nil {
			conn.mu.Lock()
			closed := conn.closes
			conn.mu.Unlock()
			if closed != 0 {
				t.Fatal("concurrent close killed mux after new client attached")
			}
			_ = c1.Close()
		} else if !errors.Is(attachErr, io.EOF) {
			t.Fatalf("unexpected concurrent attach error: %v", attachErr)
		}

		conn.mu.Lock()
		closed := conn.closes
		conn.mu.Unlock()
		if closed != 1 {
			t.Fatalf("expected single close after teardown, got %d", closed)
		}
	}
}

func TestMuxSetDeadlineUpdatesBlockedRead(t *testing.T) {
	m := &mux{
		client:       &Client{},
		done:         make(chan struct{}),
		lensTagReady: make(chan struct{}),
		startedMask:  1,
		quality:      [2]string{"2", ""},
	}
	c, err := m.attach(0, "hd", false)
	if err != nil {
		t.Fatal(err)
	}

	// Clearing a short deadline while ReadPacket is blocked must let it keep waiting.
	_ = c.SetDeadline(time.Now().Add(40 * time.Millisecond))
	got := make(chan error, 1)
	go func() {
		_, err := c.ReadPacket()
		got <- err
	}()
	time.Sleep(10 * time.Millisecond)
	_ = c.SetDeadline(time.Time{})

	time.Sleep(60 * time.Millisecond)
	select {
	case err := <-got:
		t.Fatalf("read returned early after deadline cleared: %v", err)
	default:
	}

	// Shortening deadline to the past must unblock the waiting read immediately.
	_ = c.SetDeadline(time.Now().Add(-time.Second))
	select {
	case err := <-got:
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("expected deadline exceeded, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked read did not wake on shortened deadline")
	}
}

func TestDialSharedClientRequiresMatchingTargetAndCreds(t *testing.T) {
	devPub, _, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	cliPub1, cliPriv1, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	cliPub2, cliPriv2, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	shared1, err := crypto.CalcSharedKey(hex.EncodeToString(devPub), hex.EncodeToString(cliPriv1))
	if err != nil {
		t.Fatal(err)
	}
	shared2, err := crypto.CalcSharedKey(hex.EncodeToString(devPub), hex.EncodeToString(cliPriv2))
	if err != nil {
		t.Fatal(err)
	}

	makeURL := func(user, host, model, vendor, transport, uid, pub, priv, dpub, sign, ch string) (*url.URL, url.Values) {
		q := url.Values{}
		q.Set("did", t.Name())
		q.Set("model", model)
		q.Set("vendor", vendor)
		if transport != "" {
			q.Set("transport", transport)
		}
		if uid != "" {
			q.Set("uid", uid)
		}
		q.Set("client_public", pub)
		q.Set("client_private", priv)
		q.Set("device_public", dpub)
		q.Set("sign", sign)
		q.Set("channel", ch)
		var ui *url.Userinfo
		if user != "" {
			ui = url.UserPassword(user, "cn")
		}
		return &url.URL{Scheme: "xiaomi", User: ui, Host: host, RawQuery: q.Encode()}, q
	}

	// Missing/invalid key pair fails in NewClient before touching sessions map.
	uBad, _ := makeURL("u1", "127.0.0.1", "isa.camera.hlmax", "cs2", "", "", hex.EncodeToString(cliPub1), "badhex", hex.EncodeToString(devPub), "sig1", "0")
	if _, err := NewClient(uBad.String()); err == nil {
		t.Fatal("expected error for invalid private key")
	}

	// Missing sign must not attach to an existing shared mux.
	uNoSign, qNoSign := makeURL("u1", "127.0.0.1", "isa.camera.hlmax", "cs2", "", "", hex.EncodeToString(cliPub1), hex.EncodeToString(cliPriv1), hex.EncodeToString(devPub), "", "2")
	if _, err := dialSharedClient(uNoSign, qNoSign, t.Name(), shared1); !errors.Is(err, errMuxIncompatible) {
		t.Fatalf("expected errMuxIncompatible without sign, got %v", err)
	}

	// Register an active mux under valid key1 with onClose cleanup hook.
	u1, q1 := makeURL("u1", "127.0.0.1", "isa.camera.hlmax", "cs2", "udp", "", hex.EncodeToString(cliPub1), hex.EncodeToString(cliPriv1), hex.EncodeToString(devPub), "sig1", "0")
	key1, err := makeSessionKey(u1, q1, t.Name())
	if err != nil {
		t.Fatal(err)
	}
	conn := newTestConn(make([]byte, 32))
	m := newMux(&Client{Conn: conn, key: make([]byte, 32), model: "isa.camera.hlmax"})
	c0, err := m.attach(0, "hd", false)
	if err != nil {
		t.Fatal(err)
	}
	slot := &sessionSlot{refs: 1, mux: m}
	m.mu.Lock()
	m.onClose = func() {
		slot.mu.Lock()
		if slot.mux == m {
			slot.mux = nil
		}
		slot.mu.Unlock()
		releaseSessionSlot(key1, slot)
	}
	m.mu.Unlock()
	sessionsMu.Lock()
	sessions[key1] = slot
	sessionsMu.Unlock()

	// Different user, host, model, vendor, transport, uid, or device_public must not collide.
	otherDevPub, _, _ := crypto.GenerateKey()
	for _, tc := range []struct {
		user, host, model, vendor, transport, uid, dpub string
	}{
		{"u2", "127.0.0.1", "isa.camera.hlmax", "cs2", "udp", "", hex.EncodeToString(devPub)},
		{"u1", "127.0.0.2", "isa.camera.hlmax", "cs2", "udp", "", hex.EncodeToString(devPub)},
		{"u1", "127.0.0.1", "isa.camera.other", "cs2", "udp", "", hex.EncodeToString(devPub)},
		{"u1", "127.0.0.1", "isa.camera.hlmax", "cs2", "tcp", "", hex.EncodeToString(devPub)},
		{"u1", "127.0.0.1", "isa.camera.hlmax", "tutk", "", "uid1", hex.EncodeToString(devPub)},
		{"u1", "127.0.0.1", "isa.camera.hlmax", "cs2", "udp", "", hex.EncodeToString(otherDevPub)},
	} {
		uOther, qOther := makeURL(tc.user, tc.host, tc.model, tc.vendor, tc.transport, tc.uid, hex.EncodeToString(cliPub2), hex.EncodeToString(cliPriv2), tc.dpub, "sig2", "2")
		k, err := makeSessionKey(uOther, qOther, t.Name())
		if err != nil {
			t.Fatal(err)
		}
		if k == key1 {
			t.Fatalf("distinct target %+v collided with active session key", tc)
		}
	}

	// Distinct TUTK uids must also produce distinct keys.
	uTutk1, qTutk1 := makeURL("u1", "127.0.0.1", "isa.camera.hlmax", "tutk", "", "uid1", hex.EncodeToString(cliPub1), hex.EncodeToString(cliPriv1), hex.EncodeToString(devPub), "sig1", "0")
	uTutk2, qTutk2 := makeURL("u1", "127.0.0.1", "isa.camera.hlmax", "tutk", "", "uid2", hex.EncodeToString(cliPub2), hex.EncodeToString(cliPriv2), hex.EncodeToString(devPub), "sig2", "2")
	kTutk1, err := makeSessionKey(uTutk1, qTutk1, t.Name())
	if err != nil {
		t.Fatal(err)
	}
	kTutk2, err := makeSessionKey(uTutk2, qTutk2, t.Name())
	if err != nil {
		t.Fatal(err)
	}
	if kTutk1 == kTutk2 {
		t.Fatal("distinct TUTK uids collided")
	}

	// Matching target with fresh valid client credentials shares the active mux.
	u2, q2 := makeURL("u1", "127.0.0.1", "isa.camera.hlmax", "cs2", "udp", "", hex.EncodeToString(cliPub2), hex.EncodeToString(cliPriv2), hex.EncodeToString(devPub), "sig2", "2")
	c1, err := dialSharedClient(u2, q2, t.Name(), shared2)
	if err != nil {
		t.Fatalf("matching dual lens should share active mux: %v", err)
	}
	if c1.sub == nil || c1.sub.mux != m {
		t.Fatal("expected second lens to attach to existing mux")
	}
	_ = c1.Close()

	// Closing the last subscriber must remove the entry from sessions.
	_ = c0.Close()
	sessionsMu.Lock()
	_, exists := sessions[key1]
	sessionsMu.Unlock()
	if exists {
		t.Fatal("closed session remained in global sessions registry")
	}

	// Failed dial must also leave no entry in sessions.
	uFail, qFail := makeURL("u1", "127.0.0.1:99999", "isa.camera.hlmax", "cs2", "tcp", "", hex.EncodeToString(cliPub1), hex.EncodeToString(cliPriv1), hex.EncodeToString(devPub), "sig1", "0")
	keyFail, err := makeSessionKey(uFail, qFail, t.Name())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dialSharedClient(uFail, qFail, t.Name(), shared1); err == nil {
		t.Fatal("expected dial error for unreachable endpoint")
	}
	sessionsMu.Lock()
	_, exists = sessions[keyFail]
	sessionsMu.Unlock()
	if exists {
		t.Fatal("failed dial left stale entry in global sessions registry")
	}
}

func TestMuxCloseReleasesUnderlyingBeforeSlotLock(t *testing.T) {
	conn := newTestConn(make([]byte, 32))
	m := newMux(&Client{Conn: conn, key: make([]byte, 32), model: "isa.camera.hlmax"})
	c0, err := m.attach(0, "hd", false)
	if err != nil {
		t.Fatal(err)
	}

	key := sessionKey{did: t.Name()}
	slot := &sessionSlot{refs: 1, mux: m}
	m.mu.Lock()
	m.onClose = func() {
		slot.mu.Lock()
		if slot.mux == m {
			slot.mux = nil
		}
		slot.mu.Unlock()
		releaseSessionSlot(key, slot)
	}
	m.mu.Unlock()
	sessionsMu.Lock()
	sessions[key] = slot
	sessionsMu.Unlock()

	// Hold slot.mu to simulate a concurrent reconnect dialing on the same slot.
	slot.mu.Lock()
	done := make(chan struct{})
	go func() {
		_ = c0.Close()
		close(done)
	}()

	select {
	case <-conn.closed:
	case <-time.After(time.Second):
		slot.mu.Unlock()
		t.Fatal("underlying client close was blocked by slot.mu")
	}

	slot.mu.Unlock()
	<-done

	sessionsMu.Lock()
	_, exists := sessions[key]
	sessionsMu.Unlock()
	if exists {
		t.Fatal("slot entry was not cleaned up after unlock")
	}
}
