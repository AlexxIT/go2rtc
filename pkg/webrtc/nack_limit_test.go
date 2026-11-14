package webrtc

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	pion "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

// nackPeer is a go2rtc server connection (production API, production
// interceptors) plus a plain pion receiver that counts every RTP packet per
// sequence number. A sequence number seen more than once was retransmitted.
type nackPeer struct {
	track  *Track
	remote *pion.PeerConnection
	ssrc   uint32

	mu       sync.Mutex
	received map[uint16]int
	bytes    map[uint16]int
}

func newNackPeer(t *testing.T) *nackPeer {
	t.Helper()

	api, err := NewServerAPI("", "", &Filters{Loopback: true, Networks: []string{"udp4"}})
	require.NoError(t, err)
	pc, err := api.NewPeerConnection(pion.Configuration{})
	require.NoError(t, err)
	conn := NewConn(pc)
	t.Cleanup(func() { _ = conn.Close() })

	settings := pion.SettingEngine{}
	settings.SetIncludeLoopbackCandidate(true)
	settings.SetNetworkTypes([]pion.NetworkType{pion.NetworkTypeUDP4})
	// Retransmissions reuse the original sequence number; let them through so
	// they can be counted.
	settings.DisableSRTPReplayProtection(true)
	remote, err := pion.NewAPI(pion.WithSettingEngine(settings)).NewPeerConnection(pion.Configuration{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = remote.Close() })

	p := &nackPeer{remote: remote, received: map[uint16]int{}, bytes: map[uint16]int{}}
	remote.OnTrack(func(track *pion.TrackRemote, _ *pion.RTPReceiver) {
		buf := make([]byte, 1500)
		for {
			n, _, readErr := track.Read(buf)
			if readErr != nil {
				return
			}
			var packet rtp.Packet
			if packet.Unmarshal(buf[:n]) != nil {
				continue
			}
			p.mu.Lock()
			p.received[packet.SequenceNumber]++
			p.bytes[packet.SequenceNumber] = n
			p.ssrc = packet.SSRC
			p.mu.Unlock()
		}
	})

	p.track = NewTrack("video")
	_, err = pc.AddTrack(p.track)
	require.NoError(t, err)
	_, err = pc.CreateDataChannel("test", nil)
	require.NoError(t, err)

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)
	gathered := pion.GatheringCompletePromise(pc)
	require.NoError(t, pc.SetLocalDescription(offer))
	<-gathered
	require.NoError(t, remote.SetRemoteDescription(*pc.LocalDescription()))
	answer, err := remote.CreateAnswer(nil)
	require.NoError(t, err)
	gathered = pion.GatheringCompletePromise(remote)
	require.NoError(t, remote.SetLocalDescription(answer))
	<-gathered
	require.NoError(t, pc.SetRemoteDescription(*remote.LocalDescription()))
	require.Eventually(t, func() bool {
		return pc.ConnectionState() == pion.PeerConnectionStateConnected
	}, 5*time.Second, 10*time.Millisecond)

	return p
}

// send writes count packets with the given payload size, paced so the
// loopback receive buffer does not drop originals, and waits until the
// receiver has seen all of them. Returns the sequence numbers.
func (p *nackPeer) send(t *testing.T, count, payloadSize int) []uint16 {
	t.Helper()
	payload := make([]byte, payloadSize)
	payload[0] = 0x65
	for i := 0; i < count; i++ {
		require.NoError(t, p.track.WriteRTP(96, &rtp.Packet{
			Header:  rtp.Header{Version: 2, Timestamp: uint32(i) * 3000},
			Payload: payload,
		}))
		if i%16 == 15 {
			time.Sleep(4 * time.Millisecond)
		}
	}
	require.Eventually(t, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return len(p.received) >= count*95/100
	}, 3*time.Second, 10*time.Millisecond, "receiver did not get the original packets")

	p.mu.Lock()
	defer p.mu.Unlock()
	seqs := make([]uint16, 0, len(p.received))
	for seq := range p.received {
		seqs = append(seqs, seq)
	}
	// NackPairsFromSequenceNumbers expects ascending order (no wrap in these tests).
	slices.Sort(seqs)
	return seqs
}

func (p *nackPeer) nack(t *testing.T, seqs []uint16) {
	t.Helper()
	p.mu.Lock()
	ssrc := p.ssrc
	p.mu.Unlock()
	require.NoError(t, p.remote.WriteRTCP([]rtcp.Packet{&rtcp.TransportLayerNack{
		SenderSSRC: 1, MediaSSRC: ssrc, Nacks: rtcp.NackPairsFromSequenceNumbers(seqs),
	}}))
}

// resent returns retransmitted packets and bytes (every reception beyond the first).
func (p *nackPeer) resent() (packets, bytes int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for seq, n := range p.received {
		packets += n - 1
		bytes += (n - 1) * p.bytes[seq]
	}
	return
}

// A peer that NACKs its whole history every 10 ms must not get more
// retransmitted bytes than roughly what go2rtc itself sent. Without the limiter
// the pion responder answers every NACK in full (measured ~29 MB for 1.2 MB sent).
func TestNackFloodIsBudgeted(t *testing.T) {
	p := newNackPeer(t)
	seqs := p.send(t, 1024, 1200)

	sentBytes := 0
	p.mu.Lock()
	for _, n := range p.bytes {
		sentBytes += n
	}
	p.mu.Unlock()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		p.nack(t, seqs)
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond) // let in-flight resends arrive

	resentPackets, resentBytes := p.resent()
	t.Logf("sent %d bytes, resent %d packets / %d bytes", sentBytes, resentPackets, resentBytes)

	// Budget: one window worth of own sending (capped at nackMaxResendBytesPerSec/2)
	// while the originals are inside the window, the floor afterwards.
	budget := sentBytes + 2*nackMinResendBytesPerWindow
	require.LessOrEqual(t, resentBytes, budget*12/10, "retransmissions exceed the resend budget")
	require.Positive(t, resentPackets, "flood test must still see some retransmissions")
}

// Each sequence number is retransmitted at most nackMaxResendsPerSeq times,
// however often it is requested. Payloads are small so the byte budget
// (floor) is not what limits here.
func TestNackResendsPerSequenceAreLimited(t *testing.T) {
	p := newNackPeer(t)
	seqs := p.send(t, 10, 100)

	for i := 0; i < 20; i++ {
		p.nack(t, seqs)
		// longer than the responder's own suppression interval (rtt + 5 ms)
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)

	p.mu.Lock()
	defer p.mu.Unlock()
	for _, seq := range seqs {
		resends := p.received[seq] - 1
		require.LessOrEqual(t, resends, nackMaxResendsPerSeq, "seq %d resent too often", seq)
		require.GreaterOrEqual(t, resends, 1, "seq %d was never repaired", seq)
	}
}

// The window budget must not overflow on 32-bit platforms (386, arm/v6,
// arm/v7, mipsle): 2 MiB/s * 500 ms computed in nanoseconds exceeds int32.
// A negative budget would drop every NACK. Run with GOARCH=386 to cover it.
func TestNackLimiterWindowBudgetFitsAllArchs(t *testing.T) {
	f := newNackLimiter(nackMaxResendBytesPerSec, nackMaxResendsPerSeq)
	i, err := f.NewInterceptor("")
	require.NoError(t, err)
	l := i.(*nackLimiter)
	require.EqualValues(t, nackMaxResendBytesPerSec/2, l.maxWindowBytes)
}

// A NACK the limiter empties completely (here: foreign SSRC) must not be
// re-marshalled as a header without FCI. rtcp.Unmarshal rejects such a packet,
// so any consumer of the raw bytes would lose the whole compound, including a
// PLI next to it. The cached packets keep the emptied NACK for the responder.
func TestNackLimiterRawBytesStayParsable(t *testing.T) {
	compound, err := rtcp.Marshal([]rtcp.Packet{
		&rtcp.TransportLayerNack{SenderSSRC: 1, MediaSSRC: 0xdeadbeef, Nacks: []rtcp.NackPair{{PacketID: 7}}},
		&rtcp.PictureLossIndication{SenderSSRC: 1, MediaSSRC: 0x1234},
	})
	require.NoError(t, err)

	f := newNackLimiter(nackMaxResendBytesPerSec, nackMaxResendsPerSeq)
	i, err := f.NewInterceptor("")
	require.NoError(t, err)
	reader := i.BindRTCPReader(interceptor.RTCPReaderFunc(
		func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
			return copy(b, compound), a, nil
		},
	))

	buf := make([]byte, 1500)
	n, attr, err := reader.Read(buf, nil)
	require.NoError(t, err)

	raw, err := rtcp.Unmarshal(buf[:n])
	require.NoError(t, err, "re-marshalled RTCP must stay parsable")
	require.Len(t, raw, 1)
	pli, ok := raw[0].(*rtcp.PictureLossIndication)
	require.True(t, ok, "raw bytes must carry the PLI, got %T", raw[0])
	require.EqualValues(t, 0x1234, pli.MediaSSRC)

	cached, err := attr.GetRTCPPackets(buf[:n])
	require.NoError(t, err)
	require.Len(t, cached, 2, "the responder still sees the emptied NACK")
	nack, ok := cached[0].(*rtcp.TransportLayerNack)
	require.True(t, ok)
	require.Empty(t, nack.Nacks)
}

// If the limiter empties every packet of a read, no raw bytes are left; the
// cache still carries the emptied NACK.
func TestNackLimiterRawBytesEmptyWhenAllNacksEmptied(t *testing.T) {
	compound, err := rtcp.Marshal([]rtcp.Packet{
		&rtcp.TransportLayerNack{SenderSSRC: 1, MediaSSRC: 0xdeadbeef, Nacks: []rtcp.NackPair{{PacketID: 7}}},
	})
	require.NoError(t, err)

	f := newNackLimiter(nackMaxResendBytesPerSec, nackMaxResendsPerSeq)
	i, err := f.NewInterceptor("")
	require.NoError(t, err)
	reader := i.BindRTCPReader(interceptor.RTCPReaderFunc(
		func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
			return copy(b, compound), a, nil
		},
	))

	buf := make([]byte, 1500)
	n, attr, err := reader.Read(buf, nil)
	require.NoError(t, err)
	require.Zero(t, n)

	cached, err := attr.GetRTCPPackets(buf[:n])
	require.NoError(t, err)
	require.Len(t, cached, 1)
}
