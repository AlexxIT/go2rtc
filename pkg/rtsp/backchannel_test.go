package rtsp

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

const backchannelSDP = "v=0\r\n" +
	"o=- 1 1 IN IP4 0.0.0.0\r\n" +
	"s=test\r\n" +
	"c=IN IP4 0.0.0.0\r\n" +
	"t=0 0\r\n" +
	"m=video 0 RTP/AVP 96\r\n" +
	"a=rtpmap:96 H264/90000\r\n" +
	"a=control:trackID=0\r\n" +
	"m=audio 0 RTP/AVP 0\r\n" +
	"a=rtpmap:0 PCMU/8000\r\n" +
	"a=control:trackID=1\r\n" +
	"a=sendonly\r\n"

// fakeCamera - RTSP server that records the methods of each session
type fakeCamera struct {
	ln       net.Listener
	mu       sync.Mutex
	sessions [][]string
	played   chan int    // session index after PLAY
	rtp      chan []byte // backchannel RTP received by the camera

	rejectSetup int // session index that fails on SETUP, -1 for none
}

func newFakeCamera(t *testing.T) *fakeCamera {
	ln, err := net.Listen("tcp", "localhost:0")
	require.Nil(t, err)

	cam := &fakeCamera{ln: ln, played: make(chan int, 10), rtp: make(chan []byte, 10), rejectSetup: -1}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			cam.mu.Lock()
			cam.sessions = append(cam.sessions, nil)
			i := len(cam.sessions) - 1
			cam.mu.Unlock()
			go cam.handle(conn, i)
		}
	}()
	return cam
}

func (cam *fakeCamera) methods(i int) []string {
	cam.mu.Lock()
	defer cam.mu.Unlock()
	return append([]string{}, cam.sessions[i]...)
}

func (cam *fakeCamera) handle(conn net.Conn, i int) {
	defer conn.Close()
	r := bufio.NewReader(conn)

	for {
		b, err := r.Peek(1)
		if err != nil {
			return
		}

		if b[0] == '$' {
			header := make([]byte, 4)
			if _, err = io.ReadFull(r, header); err != nil {
				return
			}
			data := make([]byte, int(header[2])<<8|int(header[3]))
			if _, err = io.ReadFull(r, data); err != nil {
				return
			}
			cam.rtp <- data
			continue
		}

		req, err := tcp.ReadRequest(r)
		if err != nil {
			return
		}

		cam.mu.Lock()
		cam.sessions[i] = append(cam.sessions[i], req.Method)
		cam.mu.Unlock()

		res := "RTSP/1.0 200 OK\r\nCSeq: " + req.Header.Get("CSeq") + "\r\n"
		switch {
		case req.Method == MethodSetup && i == cam.rejectSetup:
			res = "RTSP/1.0 453 Not Enough Bandwidth\r\nCSeq: " + req.Header.Get("CSeq") + "\r\n\r\n"
		case req.Method == MethodDescribe:
			res += "Content-Type: application/sdp\r\nContent-Length: " +
				strconv.Itoa(len(backchannelSDP)) + "\r\n\r\n" + backchannelSDP
		case req.Method == MethodSetup:
			res += "Session: " + strconv.Itoa(i+1) + ";timeout=60\r\nTransport: " +
				req.Header.Get("Transport") + "\r\n\r\n"
		default:
			res += "\r\n"
		}
		if _, err = conn.Write([]byte(res)); err != nil {
			return
		}

		if req.Method == MethodPlay {
			cam.played <- i
		}
	}
}

func waitPlayed(t *testing.T, cam *fakeCamera) int {
	select {
	case i := <-cam.played:
		return i
	case <-time.After(5 * time.Second):
		t.Fatal("PLAY timeout")
	}
	return -1
}

// TestBackchannelSeparateSession - adding a backchannel to a playing stream
// should not reconnect the main session (no glitch in the video)
func TestBackchannelSeparateSession(t *testing.T) {
	Timeout = 5 * time.Second

	cam := newFakeCamera(t)
	defer cam.ln.Close()

	client := NewClient("rtsp://" + cam.ln.Addr().String() + "/stream")
	client.Backchannel = true

	require.Nil(t, client.Dial())
	require.Nil(t, client.Describe())

	medias := client.GetMedias()
	require.Len(t, medias, 2)
	video, backchannel := medias[0], medias[1]
	require.Equal(t, core.DirectionRecvonly, video.Direction)
	require.Equal(t, core.DirectionSendonly, backchannel.Direction)

	// 1. Start the stream with video only
	_, err := client.GetTrack(video, video.Codecs[0])
	require.Nil(t, err)

	go func() { _ = client.Start() }()
	defer client.Stop()

	require.Equal(t, 0, waitPlayed(t, cam))

	// 2. Enable backchannel while playing
	mic := core.NewReceiver(
		&core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly},
		&core.Codec{Name: core.CodecPCMU, ClockRate: 8000, PayloadType: 0},
	)
	require.Nil(t, client.AddTrack(backchannel, backchannel.Codecs[0], mic))

	require.Equal(t, 1, waitPlayed(t, cam))

	// main session is not interrupted
	require.Equal(t, []string{MethodDescribe, MethodSetup, MethodPlay}, cam.methods(0))
	// backchannel session only setups the backchannel media
	require.Equal(t, []string{MethodDescribe, MethodSetup, MethodPlay}, cam.methods(1))

	// 3. Audio from the client is sent to the camera with the backchannel session
	for seq := uint16(0); ; seq++ {
		mic.WriteRTP(&rtp.Packet{
			Header:  rtp.Header{Version: 2, SequenceNumber: seq, Timestamp: uint32(seq) * 160},
			Payload: make([]byte, 160),
		})

		select {
		case data := <-cam.rtp:
			packet := &rtp.Packet{}
			require.Nil(t, packet.Unmarshal(data))
			require.Len(t, packet.Payload, 160)
			goto stop
		case <-time.After(20 * time.Millisecond):
		}

		if seq > 200 {
			t.Fatal("backchannel RTP timeout")
		}
	}

stop:
	// 4. Stop closes both sessions
	require.Nil(t, client.Stop())
	require.Eventually(t, func() bool {
		return len(cam.methods(1)) == 4
	}, time.Second, 10*time.Millisecond, fmt.Sprint(cam.methods(1)))
	require.Equal(t, MethodTeardown, cam.methods(0)[len(cam.methods(0))-1])
	require.Equal(t, MethodTeardown, cam.methods(1)[3])
}

// TestBackchannelFallback - camera doesn't accept a separate backchannel session,
// so the main session is reconnected with the backchannel (old behaviour)
func TestBackchannelFallback(t *testing.T) {
	Timeout = 5 * time.Second

	cam := newFakeCamera(t)
	cam.rejectSetup = 1
	defer cam.ln.Close()

	client := NewClient("rtsp://" + cam.ln.Addr().String() + "/stream")
	client.Backchannel = true

	require.Nil(t, client.Dial())
	require.Nil(t, client.Describe())

	video, backchannel := client.GetMedias()[0], client.GetMedias()[1]

	_, err := client.GetTrack(video, video.Codecs[0])
	require.Nil(t, err)

	go func() { _ = client.Start() }()
	defer client.Stop()

	require.Equal(t, 0, waitPlayed(t, cam))

	mic := core.NewReceiver(
		&core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly},
		&core.Codec{Name: core.CodecPCMU, ClockRate: 8000, PayloadType: 0},
	)
	require.Nil(t, client.AddTrack(backchannel, backchannel.Codecs[0], mic))

	// main session is played again after reconnect with video and backchannel
	require.Equal(t, 2, waitPlayed(t, cam))
	require.Equal(t, []string{MethodDescribe, MethodSetup, MethodSetup, MethodPlay}, cam.methods(2))
	require.Nil(t, client.backchannel)

	// failed backchannel session and old main session are closed
	require.Eventually(t, func() bool {
		return fmt.Sprint(cam.methods(1)) == fmt.Sprint([]string{MethodDescribe, MethodSetup, MethodTeardown}) &&
			fmt.Sprint(cam.methods(0)) == fmt.Sprint([]string{MethodDescribe, MethodSetup, MethodPlay, MethodTeardown})
	}, time.Second, 10*time.Millisecond, "%v %v", cam.methods(0), cam.methods(1))
}
