package rtp

import (
	"fmt"
	"net"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

// RTP implements bidirectional RTP audio as both Consumer and Producer.
//
// As Consumer: receives audio FROM camera, sends to remote RTP endpoint
// As Producer: receives audio FROM remote RTP endpoint, sends to camera backchannel
//
// The RTP endpoint always uses PCMA or PCMU for the remote side (SIP/RTP caller).
// If the camera uses a different codec (PCMU, PCMA, PCM, PCML), transcoding is
// set up automatically in AddTrack/GetTrack when the actual codec is known.
type RTP struct {
	core.Connection

	mu         sync.Mutex
	remoteAddr *net.UDPAddr
	localConn  *net.UDPConn

	// remoteCodec is what we send/receive on the RTP side (PCMA or PCMU)
	remoteCodec *core.Codec

	// For receiving from remote (backchannel audio TO camera)
	receiver *core.Receiver

	// ToRemote transcodes from the actual camera codec to remoteCodec.
	// Set up in AddTrack when the camera codec is known.
	ToRemote func([]byte) []byte

	// FromRemote transcodes from remoteCodec to the actual camera codec.
	// Set up in GetTrack when the camera codec is known.
	FromRemote func([]byte) []byte

	// Sequence number and SSRC for outgoing packets
	seqNum uint16
	ssrc   uint32
}

// NewRTP creates a bidirectional RTP endpoint.
// remote: the remote RTP endpoint (e.g. "192.168.1.200:5000")
// localPort: the local UDP port to listen on for incoming audio (backchannel)
// remoteCodec: the codec to use for the RTP side (typically PCMA or PCMU)
// Note: transcoding is set up later in AddTrack/GetTrack when camera codec is known.
func NewRTP(remote string, localPort int, remoteCodec *core.Codec) (*RTP, error) {
	remoteAddr, err := net.ResolveUDPAddr("udp", remote)
	if err != nil {
		return nil, fmt.Errorf("invalid remote address: %w", err)
	}

	localConn, err := net.ListenUDP("udp", &net.UDPAddr{Port: localPort})
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %d: %w", localPort, err)
	}

	r := &RTP{
		remoteAddr:  remoteAddr,
		localConn:   localConn,
		remoteCodec: remoteCodec,
		ssrc:        1,
	}

	r.SetProtocol("rtp")
	r.SetRemoteAddr(remote)

	go r.readLoop()

	return r, nil
}

// codecsMatch returns true if two codecs are compatible (no transcoding needed)
func codecsMatch(a, b *core.Codec) bool {
	if a == nil || b == nil {
		return true
	}
	return a.Name == b.Name && (a.ClockRate == b.ClockRate || a.ClockRate == 0 || b.ClockRate == 0)
}

// isTranscodable returns true if the codec can be used with pcm.Transcode
func isTranscodable(codec *core.Codec) bool {
	if codec == nil {
		return false
	}
	switch codec.Name {
	case core.CodecPCMA, core.CodecPCMU, core.CodecPCM, core.CodecPCML:
		return true
	}
	return false
}

// GetMedias returns media descriptions for both directions.
// We offer both PCMA and PCMU so the remote can choose (like WebRTC does).
func (r *RTP) GetMedias() []*core.Media {
	return []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs:    r.getCodecs(),
		},
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs:    r.getCodecs(),
		},
	}
}

// getCodecs returns the list of codecs we support for the remote side.
// Always includes PCMA and PCMU for maximum compatibility.
func (r *RTP) getCodecs() []*core.Codec {
	codecs := []*core.Codec{
		{Name: core.CodecPCMA, ClockRate: 8000, PayloadType: 8},
		{Name: core.CodecPCMU, ClockRate: 8000, PayloadType: 0},
	}
	// Add the configured remote codec if it's different
	if r.remoteCodec != nil && r.remoteCodec.Name != core.CodecPCMA && r.remoteCodec.Name != core.CodecPCMU {
		codecs = append(codecs, r.remoteCodec)
	}
	return codecs
}

// AddTrack is called when the camera produces audio (send direction).
// The track receives camera audio, we transcode and send to remote.
// Transcoding is set up here based on the actual camera codec from the track.
func (r *RTP) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sender := core.NewSender(media, codec)
	sender.WithParent(track)

	// Set up transcoding: track.Codec is the actual camera codec,
	// we need to convert to remoteCodec for the remote endpoint
	cameraCodec := track.Codec
	if cameraCodec != nil && !codecsMatch(cameraCodec, r.remoteCodec) {
		if isTranscodable(cameraCodec) && isTranscodable(r.remoteCodec) {
			r.ToRemote = pcm.Transcode(r.remoteCodec, cameraCodec)
			sender.Handler = func(packet *core.Packet) {
				packet.Payload = r.ToRemote(packet.Payload)
			}
		}
	}

	sender.Output = func(packet *core.Packet) {
		r.sendToRemote(packet)
	}
	sender.Start()

	r.Senders = append(r.Senders, sender)
	return nil
}

// GetTrack is called when we need to receive audio from remote (backchannel).
// Returns a receiver that accepts RTP audio and forwards to camera.
// Sets up reverse transcoding based on the codec the camera expects.
func (r *RTP) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.receiver != nil {
		return r.receiver, nil
	}

	r.receiver = core.NewReceiver(media, codec)

	// Set up reverse transcoding: remote sends remoteCodec,
	// camera expects 'codec' (the matched camera backchannel codec)
	if !codecsMatch(r.remoteCodec, codec) {
		if isTranscodable(r.remoteCodec) && isTranscodable(codec) {
			r.FromRemote = pcm.Transcode(codec, r.remoteCodec)
		}
	}

	return r.receiver, nil
}

func (r *RTP) Start() error {
	return nil
}

func (r *RTP) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.localConn != nil {
		r.localConn.Close()
		r.localConn = nil
	}

	return r.Connection.Stop()
}

// sendToRemote sends an RTP packet to the remote endpoint.
func (r *RTP) sendToRemote(packet *core.Packet) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.localConn == nil {
		return
	}

	r.seqNum++

	header := rtp.Header{
		Version:        2,
		Marker:         true,
		PayloadType:    r.getPayloadType(),
		SequenceNumber: r.seqNum,
		Timestamp:      packet.Timestamp,
		SSRC:           r.ssrc,
	}

	pkt := &rtp.Packet{
		Header:  header,
		Payload: packet.Payload,
	}

	data, err := pkt.Marshal()
	if err != nil {
		return
	}

	r.localConn.WriteToUDP(data, r.remoteAddr)
}

// readLoop reads incoming RTP packets from the remote endpoint.
// RTCP packets (PT >= 200) are silently discarded.
func (r *RTP) readLoop() {
	buf := make([]byte, 1500)
	for {
		n, _, err := r.localConn.ReadFromUDP(buf)
		if err != nil {
			return
		}

		// Discard RTCP packets (payload type >= 200)
		if n > 0 && buf[0]&0x80 != 0 {
			pt := buf[1] & 0x7F
			if pt >= 200 {
				continue
			}
		}

		pkt := &rtp.Packet{}
		if err := pkt.Unmarshal(buf[:n]); err != nil {
			continue
		}

		payload := pkt.Payload

		// Transcode from remote codec to camera codec if needed
		r.mu.Lock()
		fromRemote := r.FromRemote
		receiver := r.receiver
		r.mu.Unlock()

		if fromRemote != nil {
			payload = fromRemote(payload)
		}

		packet := &core.Packet{
			Header:  pkt.Header,
			Payload: payload,
		}

		if receiver != nil {
			receiver.Input(packet)
		}
	}
}

func (r *RTP) getPayloadType() byte {
	switch r.remoteCodec.Name {
	case core.CodecPCMU:
		return 0
	case core.CodecPCMA:
		return 8
	case core.CodecG722:
		return 9
	case core.CodecOpus:
		return 111
	default:
		return 96
	}
}
