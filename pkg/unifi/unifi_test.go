package unifi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/creds"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	retryDelay = 0 // tests check failures without the delay
	os.Exit(m.Run())
}

const cameraPath = "/proxy/protect/integration/v1/cameras/camera1"

type protect struct {
	*httptest.Server
	posts   atomic.Int32
	camera  int // status code for GET camera
	session any // JSON body for POST talkback-session
}

func newProtect(t *testing.T) *protect {
	p := &protect{camera: http.StatusOK}
	p.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "test-api-key", r.Header.Get("X-API-KEY"))
		require.Empty(t, r.URL.RawQuery)

		switch {
		case r.Method == "GET" && r.URL.Path == cameraPath:
			w.WriteHeader(p.camera)
			_, _ = w.Write([]byte(`{"featureFlags":{"hasSpeaker":true}}`))
		case r.Method == "POST" && r.URL.Path == cameraPath+"/talkback-session":
			p.posts.Add(1)
			_ = json.NewEncoder(w).Encode(p.session)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.Close)
	return p
}

func (p *protect) source() string {
	return "unifi:" + p.URL + "?camera_id=camera1&api_key=test-api-key"
}

type backchannel struct {
	core.Connection
	session *Session
	codec   *core.Codec
	audio   chan []byte // what reaches FFmpeg
	started chan struct{}
	stopped chan struct{}
}

func (b *backchannel) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	sender := core.NewSender(media, track.Codec)
	sender.Handler = func(packet *rtp.Packet) { b.audio <- packet.Payload }
	sender.HandleRTP(track)
	return nil
}

func (b *backchannel) Start() error {
	close(b.started)
	<-b.stopped
	return nil
}

func (b *backchannel) Stop() error {
	close(b.stopped)
	return nil
}

// dial - opens the client, adds a microphone track and starts it in background
func dial(t *testing.T, p *protect) (*Client, *backchannel, *core.Receiver, chan error) {
	client, err := Dial(p.source())
	require.NoError(t, err)

	bc := &backchannel{audio: make(chan []byte, 100), started: make(chan struct{}), stopped: make(chan struct{})}
	client.Backchannel = func(session *Session, codec *core.Codec) (core.Producer, error) {
		bc.session, bc.codec = session, codec
		return bc, nil
	}

	media := client.Medias[0]
	track := core.NewReceiver(media, media.Codecs[0])
	require.NoError(t, client.AddTrack(media, media.Codecs[0], track))

	done := make(chan error, 1)
	go func() { done <- client.Start() }()
	return client, bc, track, done
}

// recv - fails the test instead of hanging
func recv[T any](t *testing.T, ch <-chan T) T {
	select {
	case v := <-ch:
		return v
	case <-time.After(time.Second):
		t.Fatal("timeout")
		panic("unreachable")
	}
}

func writeRTP(track *core.Receiver, payload ...byte) {
	if payload == nil {
		payload = []byte{1, 2, 3}
	}
	track.WriteRTP(&rtp.Packet{Header: rtp.Header{Version: 2, PayloadType: 8}, Payload: payload})
}

// talk - writes to the track until the backchannel gets the payload
func talk(t *testing.T, bc *backchannel, track *core.Receiver, payload byte) {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		writeRTP(track, payload)
		select {
		case b := <-bc.audio:
			if b[0] == payload {
				return
			}
		case <-time.After(10 * time.Millisecond):
		}
	}
	t.Fatalf("payload %d does not reach the backchannel", payload)
}

func TestDial(t *testing.T) {
	p := newProtect(t)

	client, err := Dial(p.source())
	require.NoError(t, err)
	require.Equal(t, "unifi:"+p.URL+"?camera_id=camera1&api_key=***", creds.SecretString(client.URL))

	// /api/streams
	b, err := json.Marshal(client)
	require.NoError(t, err)
	require.Contains(t, creds.SecretString(string(b)), `api_key=***`)
	require.Equal(t, []*core.Media{{
		Kind:      core.KindAudio,
		Direction: core.DirectionSendonly,
		Codecs: []*core.Codec{
			{Name: core.CodecPCMA, ClockRate: 8000},
			{Name: core.CodecPCMU, ClockRate: 8000},
		},
	}}, client.Medias)

	_, err = Dial("unifi:" + p.URL + "?camera_id=camera1")
	require.EqualError(t, err, "unifi: camera_id and api_key required")
}

func TestDialErrors(t *testing.T) {
	p := newProtect(t)

	p.camera = http.StatusUnauthorized
	_, err := Dial(p.source())
	require.EqualError(t, err, "unifi: GET "+cameraPath+": 401 Unauthorized")

	p.camera = http.StatusInternalServerError
	_, err = Dial(p.source())
	require.EqualError(t, err, "unifi: GET "+cameraPath+": 500 Internal Server Error")

	source := p.source()
	p.Close()
	_, err = Dial(source)
	require.ErrorContains(t, err, "connection refused")
}

func TestDialTimeout(t *testing.T) {
	defer func(d time.Duration) { timeout = d }(timeout)
	timeout = 100 * time.Millisecond

	hang := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-hang
	}))
	defer server.Close()
	defer close(hang)

	done := make(chan error, 1)
	go func() {
		_, err := Dial("unifi:" + server.URL + "?camera_id=camera1&api_key=test-api-key")
		done <- err
	}()

	select {
	case err := <-done:
		require.ErrorContains(t, err, "context deadline exceeded")
	case <-time.After(5 * time.Second):
		t.Fatal("Dial does not return when Protect does not respond")
	}
}

func TestNoSessionWithoutAudio(t *testing.T) {
	p := newProtect(t)
	client, _, _, done := dial(t, p)

	time.Sleep(50 * time.Millisecond)
	require.NoError(t, client.Stop())
	require.NoError(t, recv(t, done))
	require.Zero(t, p.posts.Load())
}

func TestTalkback(t *testing.T) {
	p := newProtect(t)
	p.session = Session{URL: "rtp://127.0.0.1:6500", Codec: "opus", SamplingRate: 24000}
	client, bc, track, done := dial(t, p)

	writeRTP(track)
	recv(t, bc.started)

	require.EqualValues(t, 1, p.posts.Load())
	require.Equal(t, &Session{URL: "rtp://127.0.0.1:6500", Codec: "opus", SamplingRate: 24000}, bc.session)
	require.Equal(t, core.CodecPCMA, bc.codec.Name)
	talk(t, bc, track, 1)
	require.Equal(t, "rtp://127.0.0.1:6500", creds.SecretString(bc.session.URL), "session URL is no secret")

	writeRTP(track) // second packet does not open a second session

	require.NoError(t, client.Stop())
	require.NoError(t, recv(t, done))
	require.EqualValues(t, 1, p.posts.Load())
}

func TestTalkbackSessionErrors(t *testing.T) {
	for _, test := range []struct {
		session any
		err     string
	}{
		{Session{Codec: "opus", SamplingRate: 24000}, "unifi: talkback url required"},
		{Session{URL: "rtp://127.0.0.1:6500", Codec: "opus"}, "unifi: talkback samplingRate required"},
		{"bad", "json: cannot unmarshal string into Go value of type unifi.Session"},
	} {
		p := newProtect(t)
		p.session = test.session
		client, bc, track, done := dial(t, p)

		writeRTP(track)
		require.EqualError(t, recv(t, done), test.err)
		require.Nil(t, bc.session, "backchannel must not start")
		_ = client.Stop()
	}
}

func TestRequestDoesNotFollowRedirect(t *testing.T) {
	var leaked atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "" {
			leaked.Add(1)
		}
		_, _ = w.Write([]byte(`{"featureFlags":{"hasSpeaker":true}}`))
	}))
	defer other.Close()

	source := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusFound)
	})

	_, err := Dial(source)
	require.EqualError(t, err, "unifi: GET "+cameraPath+": 302 Found")
	require.Zero(t, leaked.Load(), "X-API-KEY sent to another host")
}

func newServer(t *testing.T, handler http.HandlerFunc) string {
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return "unifi:" + server.URL + "?camera_id=camera1&api_key=test-api-key"
}

func TestDialTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"featureFlags":{"hasSpeaker":true}}`))
	}))
	defer server.Close()

	// self-signed certificate is OK for an IP address, like tcp.Do
	_, err := Dial("unifi:" + server.URL + "?camera_id=camera1&api_key=test-api-key")
	require.NoError(t, err)

	// but not for a hostname
	_, err = Dial("unifi:" + strings.Replace(server.URL, "127.0.0.1", "localhost", 1) + "?camera_id=camera1&api_key=test-api-key")
	require.ErrorContains(t, err, "certificate")
}

func TestTalkbackLastMicrophoneWins(t *testing.T) {
	p := newProtect(t)
	p.session = Session{URL: "rtp://127.0.0.1:6500", Codec: "opus", SamplingRate: 24000}
	client, bc, first, done := dial(t, p)

	writeRTP(first)
	recv(t, bc.started)
	talk(t, bc, first, 1)

	// the mic page is reloaded or a second device talks
	media := client.Medias[0]
	second := core.NewReceiver(media, media.Codecs[0])
	require.NoError(t, client.AddTrack(media, media.Codecs[0], second))
	talk(t, bc, second, 2)

	// the first microphone is no longer forwarded
	time.Sleep(50 * time.Millisecond)
	for len(bc.audio) > 0 {
		<-bc.audio // drop leftovers from talk
	}
	writeRTP(first, 9)
	select {
	case b := <-bc.audio:
		t.Fatalf("first microphone still forwarded: %v", b)
	case <-time.After(100 * time.Millisecond):
	}

	// the first consumer leaves, the second keeps talking
	first.Close()
	talk(t, bc, second, 4)

	// the last consumer leaves too, nothing hangs
	second.Close()
	require.NoError(t, client.Stop())
	require.NoError(t, recv(t, done))
	require.EqualValues(t, 1, p.posts.Load(), "one talkback session")
}

func TestTalkbackMicrophoneCodecChanged(t *testing.T) {
	p := newProtect(t)
	client, _, _, _ := dial(t, p)
	defer client.Stop()

	media := client.Medias[0]
	err := client.AddTrack(media, media.Codecs[1], core.NewReceiver(media, media.Codecs[1]))
	require.EqualError(t, err, "unifi: microphone codec changed: PCMU")
}

// A malicious session URL must not become a mask pattern for logs and API output.
func TestTalkbackSessionURLIsNoSecret(t *testing.T) {
	p := newProtect(t)
	p.session = Session{URL: `"`, Codec: "opus", SamplingRate: 24000}
	client, bc, track, done := dial(t, p)

	writeRTP(track)
	recv(t, bc.started)
	require.Equal(t, `{"a":"x"}`, creds.SecretString(`{"a":"x"}`))

	require.NoError(t, client.Stop())
	require.NoError(t, recv(t, done))
}

// streams reconnects at once after Start, a failing talkback must not hammer Protect
func TestTalkbackRetryDelay(t *testing.T) {
	defer func(d time.Duration) { retryDelay = d }(retryDelay)
	retryDelay = 200 * time.Millisecond

	p := newProtect(t)
	p.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			p.posts.Add(1)
			http.Error(w, "", http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"featureFlags":{"hasSpeaker":true}}`))
	})

	// like streams.Producer.reconnect with a microphone that keeps sending
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		client, _, track, done := dial(t, p)
		writeRTP(track)
		require.Error(t, recv(t, done))
		_ = client.Stop()
	}

	require.LessOrEqual(t, p.posts.Load(), int32(6), "at most one POST per retryDelay")
}

func TestTalkbackRetryDelayStop(t *testing.T) {
	defer func(d time.Duration) { retryDelay = d }(retryDelay)
	retryDelay = time.Hour

	p := newProtect(t) // empty session -> talkback fails
	client, _, track, done := dial(t, p)
	writeRTP(track)
	time.Sleep(50 * time.Millisecond)

	require.NoError(t, client.Stop())
	require.EqualError(t, recv(t, done), "unifi: talkback url required", "Stop ends the wait")
}
