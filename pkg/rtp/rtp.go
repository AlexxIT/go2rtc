package rtp

import (
	"fmt"
	"net"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

// RTP implements bidirectional RTP as both Consumer and Producer.
//
// As Consumer: receives media FROM camera, sends to remote RTP endpoint
// As Producer: receives media FROM remote RTP endpoint, sends to camera backchannel
//
// Audio: The RTP endpoint uses PCMA or PCMU for the remote side (SIP/RTP caller).
// If the camera uses a different codec (PCMU, PCMA, PCM, PCML), transcoding is
// set up automatically in AddTrack/GetTrack when the actual codec is known.
//
// Video: The RTP endpoint passes through the camera's native video codec (H264/H265)
// with no transcoding. Only camera→caller direction is supported. RTCP Receiver
// Reports are generated to keep the remote happy.
type RTP struct {
	core.Connection

	mu         sync.Mutex
	remoteAddr *net.UDPAddr
	localConn  *net.UDPConn

	// remoteCodec is what we send/receive on the RTP side
	remoteCodec *core.Codec

	// rtcpEnabled enables RTCP RR responses (needed for video)
	rtcpEnabled bool
	// rtcpRRInterval is the interval between RR responses in packets
	rtcpRRInterval int
	// rtcpPacketCount counts packets since last RR
	rtcpPacketCount int

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

	// OnActivity is called whenever a valid RTP packet is received from the remote.
	// Used by session handlers to keep the call alive.
	OnActivity func()
}

// NewRTP creates a bidirectional RTP endpoint.
// remote: the remote RTP endpoint (e.g. "192.168.1.200:5000")
// localPort: the local UDP port to listen on for incoming RTP/backchannel
// remoteCodec: the codec to use for the RTP side
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

	// Enable RTCP handling for video codecs
	if remoteCodec != nil && remoteCodec.IsVideo() {
		r.rtcpEnabled = true
		r.rtcpRRInterval = 60 // send RR roughly every 60 packets (~few seconds)
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

// GetMedias returns media descriptions.
// For audio: bidirectional (sendonly + recvonly) with PCMA/PCMU/remote codec.
// For video: sendonly (camera→caller only) with the camera's native codec.
func (r *RTP) GetMedias() []*core.Media {
	kind := core.KindAudio
	if r.remoteCodec != nil && r.remoteCodec.IsVideo() {
		kind = core.KindVideo
	}

	if kind == core.KindVideo {
		return []*core.Media{
			{
				Kind:      core.KindVideo,
				Direction: core.DirectionSendonly,
				Codecs:    r.getCodecs(),
			},
		}
	}

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
// For audio: always includes PCMA and PCMU for maximum compatibility.
// For video: returns the camera's single video codec.
func (r *RTP) getCodecs() []*core.Codec {
	// Video: return the configured video codec only (no transcoding)
	if r.remoteCodec != nil && r.remoteCodec.IsVideo() {
		pt := r.remoteCodec.PayloadType
		if pt == 0 {
			pt = 96 // dynamic payload type
		}
		return []*core.Codec{
			{Name: r.remoteCodec.Name, ClockRate: r.remoteCodec.ClockRate, PayloadType: pt},
		}
	}

	// Audio: offer PCMA and PCMU plus the remote codec
	codecs := []*core.Codec{
		{Name: core.CodecPCMA, ClockRate: 8000, PayloadType: 8},
		{Name: core.CodecPCMU, ClockRate: 8000, PayloadType: 0},
	}
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
// For audio: RTCP packets are silently discarded.
// For video: RTCP SR packets trigger RR responses.
func (r *RTP) readLoop() {
	buf := make([]byte, 1500)
	for {
		n, _, err := r.localConn.ReadFromUDP(buf)
		if err != nil {
			return
		}

		if n < 2 {
			continue
		}

		// Check if this is an RTCP packet (payload type >= 200 in the second byte)
		if r.rtcpEnabled && buf[0]&0x80 != 0 && buf[1] >= 200 {
			r.handleRTCP(buf[:n])
			continue
		}

		pkt := &rtp.Packet{}
		if err := pkt.Unmarshal(buf[:n]); err != nil {
			continue
		}

		// Notify session handler that we received a packet (keepalive)
		if r.OnActivity != nil {
			r.OnActivity()
		}

		// For video: count packets and periodically send RTCP RR
		if r.rtcpEnabled {
			r.rtcpPacketCount++
			if r.rtcpPacketCount >= r.rtcpRRInterval {
				r.sendRTCPRR()
				r.rtcpPacketCount = 0
			}
		}

		payload := pkt.Payload

		// Transcode from remote codec to camera codec if needed (audio only)
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

// handleRTCP parses an RTCP compound packet and responds to SR with RR.
func (r *RTP) handleRTCP(data []byte) {
	// RTCP header: V(2 bits), P(1 bit), count(5 bits) in byte 0
	// PT in byte 1, length in bytes 2-3 (number of 32-bit words minus one)
	if len(data) < 4 {
		return
	}

	// Parse the RTCP compound packet
	offset := 0
	for offset+4 <= len(data) {
		ver := (data[offset] >> 6) & 0x03
		// padding := (data[offset] >> 5) & 0x01
		// count := int(data[offset] & 0x1F) // number of report blocks (unused)
		pt := data[offset+1]
		// length is in 32-bit words (minus 1)
		length := int(data[offset+2])<<8 | int(data[offset+3])
		length = (length + 1) * 4 // convert to bytes

		if ver != 2 || offset+length > len(data) {
			break
		}

		// SR (pt=200) or RR (pt=201)
		if pt == 200 && offset+28 <= len(data) {
			// Parse SR to get sender SSRC and timestamp
			ssrc := uint32(data[offset+4])<<24 | uint32(data[offset+5])<<16 | uint32(data[offset+6])<<8 | uint32(data[offset+7])
			// NTP timestamp at offset+8 (64-bit)
			// RTP timestamp at offset+16 (32-bit)
			rtpTS := uint32(data[offset+16])<<24 | uint32(data[offset+17])<<16 | uint32(data[offset+18])<<8 | uint32(data[offset+19])
			// packet count at offset+20
			// octet count at offset+24

			// Generate RR response
			r.sendRTCPRRWithSSRC(ssrc, rtpTS)
		}

		offset += length
	}
}

// sendRTCPRR sends a minimal Receiver Report.
func (r *RTP) sendRTCPRR() {
	// Send RR without a specific SR (keepalive)
	r.sendRTCPRRWithSSRC(0, 0)
}

// sendRTCPRRWithSSRC sends a Receiver Report for a specific sender.
func (r *RTP) sendRTCPRRWithSSRC(senderSSRC uint32, lastRTPTS uint32) {
	if r.localConn == nil || r.remoteAddr == nil {
		return
	}

	// Build RTCP RR packet
	// Header: V=2, P=0, RC=1 (1 report block), PT=201 (RR), length=7 (32-bit words minus 1)
	rr := make([]byte, 32)

	// Byte 0: V=2, P=0, RC=1
	rr[0] = 0x81
	// Byte 1: PT=201 (RR)
	rr[1] = 201
	// Bytes 2-3: length = 7 (8 32-bit words - 1)
	rr[2] = 0
	rr[3] = 7

	// Bytes 4-7: receiver SSRC (use our SSRC)
	rr[4] = byte(r.ssrc >> 24)
	rr[5] = byte(r.ssrc >> 16)
	rr[6] = byte(r.ssrc >> 8)
	rr[7] = byte(r.ssrc)

	// Report block (starts at byte 8):
	// Bytes 8-11: sender SSRC
	rr[8] = byte(senderSSRC >> 24)
	rr[9] = byte(senderSSRC >> 16)
	rr[10] = byte(senderSSRC >> 8)
	rr[11] = byte(senderSSRC)

	// Byte 12: fraction lost = 0
	rr[12] = 0
	// Bytes 13-15: cumulative packets lost = 0
	rr[13] = 0
	rr[14] = 0
	rr[15] = 0

	// Bytes 16-19: extended highest seq number received
	// Use the RTP timestamp from the SR as a proxy
	rr[16] = byte(lastRTPTS >> 24)
	rr[17] = byte(lastRTPTS >> 16)
	rr[18] = byte(lastRTPTS >> 8)
	rr[19] = byte(lastRTPTS)

	// Bytes 20-23: interarrival jitter = 0
	rr[20] = 0
	rr[21] = 0
	rr[22] = 0
	rr[23] = 0

	// Bytes 24-27: LSR (last SR timestamp) = 0
	rr[24] = 0
	rr[25] = 0
	rr[26] = 0
	rr[27] = 0

	// Bytes 28-31: DLSR (delay since last SR) = 0
	rr[28] = 0
	rr[29] = 0
	rr[30] = 0
	rr[31] = 0

	r.localConn.WriteToUDP(rr, r.remoteAddr)
}

// RemoteIP returns the remote IP address as a string, or empty if not set.
func (r *RTP) RemoteIP() string {
	if r.remoteAddr == nil {
		return ""
	}
	return r.remoteAddr.IP.String()
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
		// Video and other codecs use dynamic payload type (96-127)
		if r.remoteCodec != nil && r.remoteCodec.PayloadType != 0 {
			return r.remoteCodec.PayloadType
		}
		return 96
	}
}
