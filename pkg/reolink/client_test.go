package reolink

import (
	"bufio"
	"encoding/binary"
	"io"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

// fakeCamera answers the Baichuan messages the client sends
type fakeCamera struct {
	conn net.Conn
	key  []byte

	configStatus uint16      // status of the first TalkConfig
	silent       atomic.Bool // do not answer anything after the login

	mu   sync.Mutex
	cmds []uint32
}

const fakeNonce = "ABCDEF0123456789"

func startCamera(t *testing.T, cam *fakeCamera) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	cam.key = []byte(md5Modern(fakeNonce + "-secret")[:16])
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		cam.conn = conn
		cam.serve()
	}()
	return "reolink://admin:secret@" + ln.Addr().String()
}

func (f *fakeCamera) serve() {
	defer f.conn.Close()

	rd := bufio.NewReader(f.conn)
	for {
		msg, err := readRequest(rd)
		if err != nil {
			return
		}

		f.mu.Lock()
		f.cmds = append(f.cmds, msg.cmd)
		configs := f.countLocked(cmdTalkConfig)
		f.mu.Unlock()

		switch {
		case msg.cmd == cmdLogin && len(msg.body) == 0:
			xml := xmlHeader + "<body>\n<Encryption version=\"1.1\">\n<type>md5</type>\n<nonce>" + fakeNonce + "</nonce>\n</Encryption>\n</body>\n"
			f.reply(msg, classReply, 0, xorBC([]byte(xml), byte(msg.id)))
		case msg.cmd == cmdLogin:
			f.reply(msg, classZero, 200, nil)
		case f.silent.Load():
		case msg.cmd == cmdTalkAbility:
			f.reply(msg, classZero, 200, (&conn{key: f.key}).encrypt([]byte(talkAbility)))
		case msg.cmd == cmdTalkConfig && configs == 1 && f.configStatus != 0:
			f.reply(msg, classZero, f.configStatus, nil)
		case msg.cmd != cmdLogout:
			f.reply(msg, classZero, 200, nil)
		}
	}
}

func (f *fakeCamera) reply(req *message, class [2]byte, status uint16, body []byte) {
	b := append([]byte{}, magic...)
	b = binary.LittleEndian.AppendUint32(b, req.cmd)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(body)))
	b = binary.LittleEndian.AppendUint32(b, req.id)
	if class == classReply {
		b = append(b, 0x01, 0xdd, class[0], class[1])
	} else {
		b = binary.LittleEndian.AppendUint16(b, status)
		b = append(b, class[0], class[1], 0, 0, 0, 0)
	}
	_, _ = f.conn.Write(append(b, body...))
}

func (f *fakeCamera) countLocked(cmd uint32) (n int) {
	for _, c := range f.cmds {
		if c == cmd {
			n++
		}
	}
	return
}

func (f *fakeCamera) count(cmd uint32) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.countLocked(cmd)
}

// readRequest also reads the 20 byte legacy class the client uses to ask for the nonce
func readRequest(rd *bufio.Reader) (*message, error) {
	header, err := rd.Peek(20)
	if err != nil {
		return nil, err
	}
	if [2]byte(header[18:20]) != classLegacy {
		return readMessage(rd)
	}
	b := make([]byte, 20)
	if _, err = io.ReadFull(rd, b); err != nil {
		return nil, err
	}
	body := make([]byte, binary.LittleEndian.Uint32(b[8:]))
	if _, err = io.ReadFull(rd, body); err != nil {
		return nil, err
	}
	return &message{cmd: binary.LittleEndian.Uint32(b[4:]), id: binary.LittleEndian.Uint32(b[12:]), body: body}, nil
}

func setTimings(t *testing.T, keepalive, idle, ahead time.Duration) {
	k, i, a := keepaliveInterval, idleTimeout, maxAhead
	keepaliveInterval, idleTimeout, maxAhead = keepalive, idle, ahead
	t.Cleanup(func() { keepaliveInterval, idleTimeout, maxAhead = k, i, a })
}

func addTrack(t *testing.T, c *Client) *core.Receiver {
	codec := &core.Codec{Name: core.CodecPCML, ClockRate: 16000}
	track := core.NewReceiver(c.Medias[0], codec)
	require.NoError(t, c.AddTrack(c.Medias[0], codec, track))
	return track
}

// packet is 20 ms of a 440 Hz tone, 16 kHz PCM
func packet() *rtp.Packet {
	b := make([]byte, 640)
	for i := 0; i < 320; i++ {
		binary.LittleEndian.PutUint16(b[i*2:], uint16(int16(5000*math.Sin(2*math.Pi*440*float64(i)/16000))))
	}
	return &rtp.Packet{Payload: b}
}

// silence is 20 ms of digital silence, as a muted browser microphone sends it
func silence() *rtp.Packet {
	return &rtp.Packet{Payload: make([]byte, 640)}
}

func TestTalkSession(t *testing.T) {
	setTimings(t, time.Second, 300*time.Millisecond, time.Second)
	cam := &fakeCamera{}
	c, err := Dial(startCamera(t, cam))
	require.NoError(t, err)
	require.Equal(t, 16000, c.cfg.sampleRate)

	var mu sync.Mutex
	var errs []error
	c.OnError = func(err error) { mu.Lock(); errs = append(errs, err); mu.Unlock() }
	track := addTrack(t, c)

	started := make(chan error, 1)
	go func() { started <- c.Start() }()

	// 10 packets = 3200 samples, a Talk message needs 4096
	for i := 0; i < 10; i++ {
		track.Input(packet())
	}
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, 1, cam.count(cmdTalkConfig))
	require.Equal(t, 0, cam.count(cmdTalk))

	for i := 0; i < 3; i++ {
		track.Input(packet())
	}
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, 1, cam.count(cmdTalk))

	// after the idle timeout the rest is flushed, and the reset follows once the camera played it
	time.Sleep(time.Second)
	require.Equal(t, 2, cam.count(cmdTalk))
	require.Equal(t, 1, cam.count(cmdTalkReset))

	require.NoError(t, c.Stop())
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
	mu.Lock()
	require.Empty(t, errs)
	mu.Unlock()
	c.mu.Lock()
	require.Equal(t, 13*640, c.Send)
	c.mu.Unlock()
}

func TestTalkConfig422(t *testing.T) {
	setTimings(t, time.Second, time.Second, time.Second)
	cam := &fakeCamera{configStatus: 422}
	c, err := Dial(startCamera(t, cam))
	require.NoError(t, err)
	track := addTrack(t, c)
	go func() { _ = c.Start() }()

	track.Input(packet())
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, 2, cam.count(cmdTalkConfig))
	require.Equal(t, 1, cam.count(cmdTalkReset))
	require.NoError(t, c.Stop())
}

// streams calls Stop with the stream locked, so it must not wait for the camera
func TestStopDoesNotBlock(t *testing.T) {
	setTimings(t, time.Second, time.Second, time.Second)
	cam := &fakeCamera{}
	c, err := Dial(startCamera(t, cam))
	require.NoError(t, err)
	track := addTrack(t, c)
	go func() { _ = c.Start() }()

	for i := 0; i < 13; i++ {
		track.Input(packet())
	}
	time.Sleep(100 * time.Millisecond)
	cam.silent.Store(true)

	start := time.Now()
	require.NoError(t, c.Stop())
	require.NoError(t, c.Stop())
	require.Less(t, time.Since(start), 50*time.Millisecond)

	// the camera still gets the rest of the audio and the reset
	time.Sleep(1500 * time.Millisecond)
	require.Equal(t, 2, cam.count(cmdTalk))
	require.Equal(t, 1, cam.count(cmdTalkReset))
}

func TestTwoTracks(t *testing.T) {
	setTimings(t, time.Second, time.Second, time.Second)
	cam := &fakeCamera{}
	c, err := Dial(startCamera(t, cam))
	require.NoError(t, err)
	track1, track2 := addTrack(t, c), addTrack(t, c)
	go func() { _ = c.Start() }()

	for i := 0; i < 7; i++ {
		track1.Input(packet())
		track2.Input(packet())
	}
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, 1, cam.count(cmdTalkConfig))
	require.NoError(t, c.Stop())
}

// a source faster than real time is slowed down to maxAhead
func TestPacing(t *testing.T) {
	setTimings(t, time.Second, time.Second, 100*time.Millisecond)
	cam := &fakeCamera{}
	c, err := Dial(startCamera(t, cam))
	require.NoError(t, err)

	samples := make([]int16, 16000) // one second of a tone at once
	for i := range samples {
		samples[i] = int16(5000 * math.Sin(2*math.Pi*440*float64(i)/16000))
	}
	start := time.Now()
	require.NoError(t, c.push(samples, 0))
	require.Greater(t, time.Since(start), 350*time.Millisecond)
	require.NoError(t, c.Stop())
}

// a camera that stops answering is detected by the read deadline, and the socket is closed
func TestDeadCamera(t *testing.T) {
	setTimings(t, 50*time.Millisecond, time.Second, time.Second)
	cam := &fakeCamera{}
	c, err := Dial(startCamera(t, cam))
	require.NoError(t, err)
	cam.silent.Store(true)

	done := make(chan error, 1)
	go func() { done <- c.Start() }()
	select {
	case err = <-done:
		var ne net.Error
		require.ErrorAs(t, err, &ne)
		require.True(t, ne.Timeout())
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return")
	}
	require.ErrorIs(t, c.conn.Conn.Close(), net.ErrClosed)
	require.NoError(t, c.Stop())
}

func TestRoundTripErrors(t *testing.T) {
	a, b := net.Pipe()
	c := &conn{Conn: a, readTimeout: time.Second, waiters: map[uint32]chan *message{}, done: make(chan struct{})}
	go c.readLoop()

	cam := &fakeCamera{conn: b}
	go func() {
		for {
			msg, err := readMessage(b)
			if err != nil {
				return
			}
			msg.cmd++ // reply with a wrong cmd
			cam.reply(msg, classZero, 200, nil)
		}
	}()

	_, err := c.roundTrip(cmdLinkType, c.nextID(hostChannel), classModern, nil, nil)
	require.ErrorContains(t, err, "has cmd")

	_ = b.Close()
	<-c.done
	_, err = c.roundTrip(cmdLinkType, c.nextID(hostChannel), classModern, nil, nil)
	require.Error(t, err)
}

func TestWrongChannel(t *testing.T) {
	for _, s := range []string{"-1", "100", "x"} {
		_, err := Dial("reolink://admin:secret@127.0.0.1:1?channel=" + s)
		require.ErrorContains(t, err, "wrong channel")
	}
}

// a muted microphone sends silence: that must not open a session, and it ends an open one
func TestSilence(t *testing.T) {
	setTimings(t, time.Second, 300*time.Millisecond, time.Second)
	cam := &fakeCamera{}
	c, err := Dial(startCamera(t, cam))
	require.NoError(t, err)
	track := addTrack(t, c)
	go func() { _ = c.Start() }()

	for i := 0; i < 20; i++ {
		track.Input(silence())
	}
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, 0, cam.count(cmdTalkConfig))

	track.Input(packet())
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, 1, cam.count(cmdTalkConfig))

	// silence keeps coming, the session still closes after the idle timeout
	for i := 0; i < 50; i++ {
		track.Input(silence())
		time.Sleep(20 * time.Millisecond)
	}
	require.Equal(t, 1, cam.count(cmdTalkReset))
	require.NoError(t, c.Stop())
}
