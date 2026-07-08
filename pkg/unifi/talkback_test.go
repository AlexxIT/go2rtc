package unifi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/creds"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestDialTalkbackURLAndSecret(t *testing.T) {
	apiKey := "unifi-talkback-secret-key"
	server, _ := newProtectServer(t, true, TalkbackSession{})
	defer server.Close()

	rawURL := SchemeTalkback + ":" + server.URL + "?camera_id=camera1&api_key=" + apiKey
	client, err := DialTalkback(rawURL)
	require.NoError(t, err)
	defer client.Stop()

	require.Equal(t, strings.ReplaceAll(rawURL, apiKey, "***"), creds.SecretString(rawURL))

	b, err := json.Marshal(client)
	require.NoError(t, err)
	require.NotContains(t, string(b), apiKey)
	require.Contains(t, string(b), "api_key=***")
}

func TestRegisterTalkbackSecrets(t *testing.T) {
	apiKey := "unifi-register-secret-key"
	rawURL := SchemeTalkback + ":https://192.168.1.1?camera_id=camera1&api_key=" + apiKey

	RegisterTalkbackSecrets(rawURL)

	require.NotContains(t, creds.SecretString(rawURL), apiKey)
	require.Contains(t, creds.SecretString(rawURL), "api_key=***")
}

func TestDialTalkbackAdvertisesMediaWithSpeaker(t *testing.T) {
	server, _ := newProtectServer(t, true, TalkbackSession{})
	defer server.Close()

	client, err := DialTalkback(SchemeTalkback + ":" + server.URL + "?camera_id=camera1&api_key=key-with-speaker")
	require.NoError(t, err)
	defer client.Stop()

	require.Len(t, client.Medias, 1)
	require.Equal(t, core.KindAudio, client.Medias[0].Kind)
	require.Equal(t, core.DirectionSendonly, client.Medias[0].Direction)
	require.Equal(t, []*core.Codec{
		{
			Name:        core.CodecOpus,
			ClockRate:   48000,
			Channels:    2,
			PayloadType: 111,
		},
		{
			Name:        core.CodecPCMU,
			ClockRate:   8000,
			PayloadType: 0,
		},
		{
			Name:        core.CodecPCMA,
			ClockRate:   8000,
			PayloadType: 8,
		},
	}, client.Medias[0].Codecs)
}

func TestDialTalkbackDoesNotAdvertiseMediaWithoutSpeaker(t *testing.T) {
	server, _ := newProtectServer(t, false, TalkbackSession{})
	defer server.Close()

	client, err := DialTalkback(SchemeTalkback + ":" + server.URL + "?camera_id=camera1&api_key=key-no-speaker")
	require.NoError(t, err)
	defer client.Stop()

	require.Empty(t, client.Medias)
}

func TestTalkbackNoSessionDuringPassiveProbe(t *testing.T) {
	server, posts := newProtectServer(t, true, TalkbackSession{
		URL:          "rtp://127.0.0.1:6000",
		Codec:        "opus",
		SamplingRate: 24000,
	})
	defer server.Close()

	client, err := DialTalkback(SchemeTalkback + ":" + server.URL + "?camera_id=camera1&api_key=key-passive")
	require.NoError(t, err)

	media := client.Medias[0]
	track := core.NewReceiver(&core.Media{
		Kind:      core.KindAudio,
		Direction: core.DirectionRecvonly,
	}, media.Codecs[0])
	require.NoError(t, client.AddTrack(media, media.Codecs[0], track))

	done := make(chan error, 1)
	go func() {
		done <- client.Start()
	}()

	time.Sleep(50 * time.Millisecond)
	require.Zero(t, posts.Load())

	require.NoError(t, client.Stop())
	require.NoError(t, <-done)
	require.Zero(t, posts.Load())
}

func TestTalkbackStartsOnFirstRTPAndStopsResources(t *testing.T) {
	restore, ffmpeg := fakeFFmpeg(t)
	defer restore()

	server, posts := newProtectServer(t, true, TalkbackSession{
		URL:          "rtp://127.0.0.1:6500",
		Codec:        "opus",
		SamplingRate: 24000,
	})
	defer server.Close()

	client, err := DialTalkback(SchemeTalkback + ":" + server.URL + "?camera_id=camera1&api_key=key-active")
	require.NoError(t, err)

	media := client.Medias[0]
	track := core.NewReceiver(&core.Media{
		Kind:      core.KindAudio,
		Direction: core.DirectionRecvonly,
	}, media.Codecs[0])
	require.NoError(t, client.AddTrack(media, media.Codecs[0], track))

	done := make(chan error, 1)
	go func() {
		done <- client.Start()
	}()

	time.Sleep(50 * time.Millisecond)
	require.Zero(t, posts.Load())
	require.Empty(t, ffmpeg.Commands())

	track.WriteRTP(&rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    111,
			SequenceNumber: 1,
			Timestamp:      960,
		},
		Payload: []byte{0x01, 0x02, 0x03},
	})

	require.Eventually(t, func() bool {
		return posts.Load() == 1 && len(ffmpeg.Commands()) == 1 && ffmpeg.writer.writeCount.Load() == 1
	}, time.Second, 10*time.Millisecond)

	cmd := ffmpeg.Commands()[0]
	require.Contains(t, cmd.command, "-c:a libopus")
	require.Contains(t, cmd.command, "-ar:a 24000")
	require.Contains(t, cmd.command, "-ac:a 1")
	require.Contains(t, cmd.command, "-f rtp rtp://127.0.0.1:6500")
	require.Contains(t, cmd.stdin.String(), "m=audio 45000 RTP/AVP 111")
	require.Contains(t, cmd.stdin.String(), "a=rtpmap:111 opus/48000/2")

	require.NoError(t, client.Stop())
	require.NoError(t, <-done)

	require.True(t, cmd.closed.Load())
	require.True(t, ffmpeg.writer.closed.Load())
}

func TestTalkbackRejectsUnsupportedCodec(t *testing.T) {
	restore, ffmpeg := fakeFFmpeg(t)
	defer restore()

	server, _ := newProtectServer(t, true, TalkbackSession{
		URL:          "rtp://127.0.0.1:6500",
		Codec:        "aac",
		SamplingRate: 16000,
	})
	defer server.Close()

	client, err := DialTalkback(SchemeTalkback + ":" + server.URL + "?camera_id=camera1&api_key=key-unsupported")
	require.NoError(t, err)
	defer client.Stop()

	media := client.Medias[0]
	track := core.NewReceiver(&core.Media{
		Kind:      core.KindAudio,
		Direction: core.DirectionRecvonly,
	}, media.Codecs[0])
	require.NoError(t, client.AddTrack(media, media.Codecs[0], track))

	done := make(chan error, 1)
	go func() {
		done <- client.Start()
	}()

	track.WriteRTP(&rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    111,
			SequenceNumber: 1,
			Timestamp:      960,
		},
		Payload: []byte{0x01, 0x02, 0x03},
	})

	require.Eventually(t, func() bool {
		return len(done) == 1
	}, time.Second, 10*time.Millisecond)

	err = <-done
	require.EqualError(t, err, "unifi: unsupported talkback codec: aac")
	require.Empty(t, ffmpeg.Commands())
}

func TestBuildFFmpegCommandOpus(t *testing.T) {
	command := buildFFmpegCommand("rtp://10.0.0.2:4444", 24000)

	require.Contains(t, command, "ffmpeg -hide_banner -loglevel error")
	require.Contains(t, command, "-protocol_whitelist file,pipe,udp,rtp")
	require.Contains(t, command, "-c:a libopus")
	require.Contains(t, command, "-application:a lowdelay")
	require.Contains(t, command, "-ar:a 24000")
	require.Contains(t, command, "-ac:a 1")
	require.Contains(t, command, "-f rtp rtp://10.0.0.2:4444")
}

func TestBuildInputSDPG711(t *testing.T) {
	pcmu := buildInputSDP(&core.Codec{Name: core.CodecPCMU, ClockRate: 8000}, 45000)
	require.Contains(t, pcmu, "m=audio 45000 RTP/AVP 0")
	require.Contains(t, pcmu, "a=rtpmap:0 PCMU/8000")

	pcma := buildInputSDP(&core.Codec{Name: core.CodecPCMA, ClockRate: 8000, PayloadType: 8}, 45002)
	require.Contains(t, pcma, "m=audio 45002 RTP/AVP 8")
	require.Contains(t, pcma, "a=rtpmap:8 PCMA/8000")
}

func TestProtectResponseLogRedactsSessionURL(t *testing.T) {
	registerSecretURL("rtp://127.0.0.1:6500")
	sessionURL := "rtp://127.0.0.1:6500/talkback?token=unifi-session-token&other=1"
	registerSecretURL(sessionURL)

	body := strings.ReplaceAll(`{"url":"`+sessionURL+`"}`, "/", `\/`)
	body = strings.ReplaceAll(body, "&", `\u0026`)
	redacted := creds.SecretString(body)

	require.NotContains(t, redacted, sessionURL)
	require.NotContains(t, redacted, "unifi-session-token")
	require.Contains(t, redacted, "***")
}

func newProtectServer(t *testing.T, hasSpeaker bool, session TalkbackSession) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	var posts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NotEmpty(t, r.Header.Get("X-API-KEY"))
		require.Empty(t, r.URL.RawQuery)

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/proxy/protect/integration/v1/cameras/camera1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"featureFlags": map[string]any{
					"hasSpeaker": hasSpeaker,
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/proxy/protect/integration/v1/cameras/camera1/talkback-session":
			posts.Add(1)
			_ = json.NewEncoder(w).Encode(session)
		default:
			http.NotFound(w, r)
		}
	}))

	return server, &posts
}

func fakeFFmpeg(t *testing.T) (func(), *fakeFFmpegState) {
	t.Helper()

	oldCommand := newFFmpegCommand
	oldReserve := reserveRTPPort
	oldDial := dialRTPPort

	state := &fakeFFmpegState{
		writer: &fakeRTPWriter{},
	}

	newFFmpegCommand = func(command string) ffmpegCommand {
		cmd := &fakeCommand{
			command: command,
			done:    make(chan struct{}),
		}
		state.mu.Lock()
		state.commands = append(state.commands, cmd)
		state.mu.Unlock()
		return cmd
	}
	reserveRTPPort = func() (int, error) {
		return 45000, nil
	}
	dialRTPPort = func(port int) (rtpWriter, error) {
		require.Equal(t, 45000, port)
		return state.writer, nil
	}

	return func() {
		newFFmpegCommand = oldCommand
		reserveRTPPort = oldReserve
		dialRTPPort = oldDial
	}, state
}

type fakeFFmpegState struct {
	mu       sync.Mutex
	commands []*fakeCommand
	writer   *fakeRTPWriter
}

func (s *fakeFFmpegState) Commands() []*fakeCommand {
	s.mu.Lock()
	defer s.mu.Unlock()

	commands := make([]*fakeCommand, len(s.commands))
	copy(commands, s.commands)
	return commands
}

type fakeCommand struct {
	command string
	stdin   bytes.Buffer
	done    chan struct{}
	once    sync.Once
	started atomic.Bool
	closed  atomic.Bool
}

func (c *fakeCommand) StdinPipe() (io.WriteCloser, error) {
	return nopWriteCloser{&c.stdin}, nil
}

func (c *fakeCommand) Start() error {
	c.started.Store(true)
	return nil
}

func (c *fakeCommand) Wait() error {
	<-c.done
	return nil
}

func (c *fakeCommand) Close() error {
	c.closed.Store(true)
	c.once.Do(func() {
		close(c.done)
	})
	return nil
}

type nopWriteCloser struct {
	io.Writer
}

func (n nopWriteCloser) Close() error {
	return nil
}

type fakeRTPWriter struct {
	closed     atomic.Bool
	writeCount atomic.Int64
	buf        bytes.Buffer
}

func (w *fakeRTPWriter) Write(b []byte) (int, error) {
	w.writeCount.Add(1)
	return w.buf.Write(b)
}

func (w *fakeRTPWriter) Close() error {
	w.closed.Store(true)
	return nil
}

func (w *fakeRTPWriter) SetWriteDeadline(time.Time) error {
	return nil
}
