package webrtc

import (
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

// TestKeyframeRequest - the browser asks for a keyframe with PLI and FIR, the
// request goes to the source of that track only
func TestKeyframeRequest(t *testing.T) {
	api, err := NewAPI()
	require.Nil(t, err)

	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err)

	conn := NewConn(pc)
	conn.Mode = core.ModePassiveConsumer
	defer conn.Close()

	browser, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err)
	defer browser.Close()

	connected := make(chan struct{})
	browser.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			close(connected)
		}
	})

	for range 2 {
		_, err = browser.AddTransceiverFromKind(
			webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly},
		)
		require.Nil(t, err)
	}

	require.Nil(t, conn.SetOffer(createOffer(t, browser)))

	// one camera per track
	requests := map[string]chan struct{}{}
	for _, mid := range []string{"0", "1"} {
		media := conn.getMedia(mid, core.DirectionSendonly)
		require.NotNil(t, media)
		codec := getCodec(media, core.CodecH264)
		require.NotNil(t, codec)

		camera := core.NewReceiver(
			&core.Media{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
			&core.Codec{Name: core.CodecH264, ClockRate: 90000, PayloadType: 96},
		)
		ch := make(chan struct{}, 100)
		camera.OnKeyframe(func() { ch <- struct{}{} })
		require.Nil(t, conn.AddTrack(media, codec, camera))

		requests[mid] = ch
	}

	answer, err := conn.GetCompleteAnswer(nil, nil)
	require.Nil(t, err)
	require.Nil(t, browser.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer}))

	// no media is needed, the browser sends RTCP for the negotiated SSRC
	select {
	case <-connected:
	case <-time.After(10 * time.Second):
		t.Fatal("connection timeout")
	}
	ssrc := map[string]uint32{}
	for mid := range requests {
		track := conn.GetSenderTrack(mid)
		require.Eventually(t, func() bool {
			track.mu.Lock()
			defer track.mu.Unlock()
			ssrc[mid] = track.ssrc
			return track.ssrc != 0
		}, 5*time.Second, 10*time.Millisecond)
	}

	// RTCP is not retransmitted, so send until the camera is asked
	request := func(mid string, pkt rtcp.Packet) {
		for n := 0; ; n++ {
			require.Less(t, n, 100, "no keyframe request for %T", pkt)
			require.Nil(t, browser.WriteRTCP([]rtcp.Packet{pkt}))

			select {
			case <-requests[mid]:
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
	}

	request("0", &rtcp.PictureLossIndication{MediaSSRC: ssrc["0"]})
	time.Sleep(50 * time.Millisecond)
	require.Empty(t, requests["1"], "PLI for track 0 asked the camera of track 1")

	request("1", &rtcp.FullIntraRequest{FIR: []rtcp.FIREntry{{SSRC: ssrc["1"]}}})
}

// rtcpScript answers the RTCP reads of a real sender, inner is the reader of
// the connection
type rtcpScript func(b []byte, a interceptor.Attributes, inner interceptor.RTCPReader) (int, interceptor.Attributes, error)

func (s rtcpScript) NewInterceptor(string) (interceptor.Interceptor, error) {
	return &scriptedRTCP{read: s}, nil
}

type scriptedRTCP struct {
	interceptor.NoOp
	read rtcpScript
}

func (s *scriptedRTCP) BindRTCPReader(inner interceptor.RTCPReader) interceptor.RTCPReader {
	return interceptor.RTCPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		return s.read(b, a, inner)
	})
}

// newRTCPSender connects a sendonly track to a local peer and returns its
// sender, the factories wrap the RTCP reader of the sender in this order
func newRTCPSender(t *testing.T, track webrtc.TrackLocal, factories ...interceptor.Factory) (*webrtc.RTPSender, *webrtc.PeerConnection) {
	m := &webrtc.MediaEngine{}
	require.Nil(t, m.RegisterDefaultCodecs())
	registry := &interceptor.Registry{}
	for _, f := range factories {
		registry.Add(f)
	}
	settings := webrtc.SettingEngine{}
	settings.SetIncludeLoopbackCandidate(true)
	settings.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})

	pc, err := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(registry), webrtc.WithSettingEngine(settings)).NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err)
	t.Cleanup(func() { _ = pc.Close() })
	tr, err := pc.AddTransceiverFromTrack(track, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly})
	require.Nil(t, err)

	remote, err := webrtc.NewAPI(webrtc.WithSettingEngine(settings)).NewPeerConnection(webrtc.Configuration{})
	require.Nil(t, err)
	t.Cleanup(func() { _ = remote.Close() })

	require.Nil(t, remote.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: createOffer(t, pc)}))
	answer, err := remote.CreateAnswer(nil)
	require.Nil(t, err)
	gathered := webrtc.GatheringCompletePromise(remote)
	require.Nil(t, remote.SetLocalDescription(answer))
	<-gathered
	require.Nil(t, pc.SetRemoteDescription(*remote.LocalDescription()))
	return tr.Sender(), pc
}

// readUntilClose runs readRTCP, closes the connection when drained is closed
// and waits until the loop ended
func readUntilClose(t *testing.T, sender *webrtc.RTPSender, pc *webrtc.PeerConnection, drained <-chan struct{}) {
	done := make(chan struct{})
	go func() {
		readRTCP(sender)
		close(done)
	}()
	select {
	case <-drained:
	case <-time.After(10 * time.Second):
		t.Fatal("RTCP script not read")
	}
	require.Nil(t, pc.Close())
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("readRTCP did not end on close")
	}
}

// rtcpReads replays errs (nil = PLI packet), then closes drained and passes
// the reads to the connection
func rtcpReads(t *testing.T, errs ...error) (rtcpScript, *[]error, <-chan struct{}) {
	pli, err := rtcp.Marshal([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: 1}})
	require.Nil(t, err)

	drained := make(chan struct{})
	read := func(b []byte, a interceptor.Attributes, inner interceptor.RTCPReader) (int, interceptor.Attributes, error) {
		if len(errs) == 0 {
			select {
			case <-drained:
			default:
				close(drained)
			}
			return inner.Read(b, a)
		}
		err := errs[0]
		errs = errs[1:]
		if err != nil {
			return 0, nil, err
		}
		return copy(b, pli), a, nil
	}
	return read, &errs, drained
}

// TestReadRTCPSkipsBrokenPackets - a failed read must not end the loop,
// a later PLI still reaches the handler
func TestReadRTCPSkipsBrokenPackets(t *testing.T) {
	read, left, drained := rtcpReads(t, errors.New("broken packet"), io.ErrShortBuffer, nil)

	var calls int
	sender, pc := newRTCPSender(t, newKeyframeTrack(func() { calls++ }), read)
	readUntilClose(t, sender, pc, drained)
	require.Equal(t, 1, calls)
	require.Empty(t, *left)
}

// TestReadRTCPWithoutHandler - the NACK responder needs the reads, so the loop
// consumes packets until the connection is closed, also without a handler
func TestReadRTCPWithoutHandler(t *testing.T) {
	read, left, drained := rtcpReads(t, nil, nil, errors.New("broken packet"), nil)

	track, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "go2rtc")
	require.Nil(t, err)
	sender, pc := newRTCPSender(t, track, read)
	readUntilClose(t, sender, pc, drained)
	require.Empty(t, *left)
}

// newKeyframeTrack returns a track whose source calls handler on a keyframe request
func newKeyframeTrack(handler func()) *Track {
	camera := core.NewReceiver(nil, &core.Codec{})
	camera.OnKeyframe(handler)
	sender := core.NewSender(nil, &core.Codec{})
	sender.Bind(camera)

	track := NewTrack("video")
	track.setSender(sender)
	return track
}

// TestReadRTCPLateSender - a sender set after the loop started gets later PLI
// (ex. AddTrack of a backchannel after the connection)
func TestReadRTCPLateSender(t *testing.T) {
	pli, err := rtcp.Marshal([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: 1}})
	require.Nil(t, err)

	packets := make(chan []byte)
	drained := make(chan struct{})
	read := func(b []byte, a interceptor.Attributes, inner interceptor.RTCPReader) (int, interceptor.Attributes, error) {
		p, ok := <-packets
		if !ok {
			select {
			case <-drained:
			default:
				close(drained)
			}
			return inner.Read(b, a)
		}
		return copy(b, p), a, nil
	}

	track := NewTrack("video")
	sender, pc := newRTCPSender(t, track, rtcpScript(read))
	// the first PLI may still meet the new sender, so count instead of expecting one
	var calls atomic.Int32
	go func() {
		packets <- pli // the loop is running, no sender yet
		track.setSender(newKeyframeTrack(func() { calls.Add(1) }).sender)
		packets <- pli
		close(packets)
	}()
	readUntilClose(t, sender, pc, drained)

	require.Positive(t, calls.Load(), "late sender got no keyframe request")
}

// TestReadRTCPStopsAfterTooManyErrors - a reader that only fails ends the loop
func TestReadRTCPStopsAfterTooManyErrors(t *testing.T) {
	var reads int
	read := func([]byte, interceptor.Attributes, interceptor.RTCPReader) (int, interceptor.Attributes, error) {
		reads++
		return 0, nil, errors.New("broken packet")
	}
	sender, _ := newRTCPSender(t, NewTrack("video"), rtcpScript(read))
	readRTCP(sender)
	require.Equal(t, maxRTCPReadErrors, reads)
}

// TestReadRTCPResetsErrorsAfterRead - only failed reads in a row end the loop,
// a good read in between starts the count again
func TestReadRTCPResetsErrorsAfterRead(t *testing.T) {
	var seq []error
	for range 2 {
		for range maxRTCPReadErrors - 1 {
			seq = append(seq, errors.New("broken packet"))
		}
		seq = append(seq, nil)
	}
	read, left, _ := rtcpReads(t, append(seq, io.EOF)...)

	var calls int
	sender, _ := newRTCPSender(t, newKeyframeTrack(func() { calls++ }), read)
	readRTCP(sender)
	require.Equal(t, 2, calls)
	require.Empty(t, *left)
}

// TestReadRTCPAfterNackLimiter - the limiter empties a NACK (foreign SSRC), a
// PLI in the same compound still reaches the keyframe reader
func TestReadRTCPAfterNackLimiter(t *testing.T) {
	compound, err := rtcp.Marshal([]rtcp.Packet{
		&rtcp.TransportLayerNack{SenderSSRC: 1, MediaSSRC: 0xdeadbeef, Nacks: []rtcp.NackPair{{PacketID: 7}}},
		&rtcp.PictureLossIndication{SenderSSRC: 1, MediaSSRC: 0x1234},
	})
	require.Nil(t, err)

	drained := make(chan struct{})
	var reads int
	read := func(b []byte, a interceptor.Attributes, inner interceptor.RTCPReader) (int, interceptor.Attributes, error) {
		if reads++; reads > 1 {
			if reads == 2 {
				close(drained)
			}
			return inner.Read(b, a)
		}
		return copy(b, compound), a, nil
	}

	var calls int
	// the limiter wraps the script, like it wraps the connection in production
	limiter := newNackLimiter(nackMaxResendBytesPerSec, nackMaxResendsPerSeq)
	sender, pc := newRTCPSender(t, newKeyframeTrack(func() { calls++ }), rtcpScript(read), limiter)
	readUntilClose(t, sender, pc, drained)
	require.Equal(t, 1, calls)
}
