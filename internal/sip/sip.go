package sip

import (
	"crypto/rand"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/rtp"
	"github.com/pion/sdp/v3"
)

type Config struct {
	Enable bool   `yaml:"enable"`
	Port   int    `yaml:"port"`
	Stream string `yaml:"stream"`
}

var cfg *Config

type session struct {
	callID    string
	tag       string // To tag from our 200 OK — reused in all subsequent responses
	rtp       *rtp.RTP
	stream    *streams.Stream
	createdAt time.Time
	localIP   string // for Contact header
	localPort int    // for Contact header
}

var (
	sessions   = map[string]*session{}
	sessionsMu sync.Mutex
	nextPort   = 10000
	portMu     sync.Mutex
)

func Init() {
	var c struct{ SIP Config `yaml:"sip"` }
	app.LoadConfig(&c)
	if !c.SIP.Enable {
		return
	}
	if c.SIP.Port == 0 {
		c.SIP.Port = 5060
	}
	cfg = &c.SIP
	log := app.GetLogger("sip")
	log.Info().Int("port", cfg.Port).Msg("[sip] starting")
	go listen(cfg.Port)
	go cleanupLoop()
}

func allocPort() int {
	portMu.Lock()
	defer portMu.Unlock()
	p := nextPort
	nextPort += 2
	if nextPort > 65000 {
		nextPort = 10000
	}
	return p
}

// cleanupLoop reaps stale sessions every 30s. If a call ends without BYE
// (network drop, device crash), the session and its RTP port are freed.
func cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		sessionsMu.Lock()
		now := time.Now()
		for id, s := range sessions {
			if now.Sub(s.createdAt) > 2*time.Minute {
				log := app.GetLogger("sip")
				log.Info().Str("call_id", id).Msg("[sip] reaping stale session")
				// RemoveConsumer calls Stop() internally
				if s.stream != nil {
					s.stream.RemoveConsumer(s.rtp)
				}
				delete(sessions, id)
			}
		}
		sessionsMu.Unlock()
	}
}

func listen(port int) {
	log := app.GetLogger("sip")
	addr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Error().Err(err).Msg("[sip] listen failed")
		return
	}
	defer conn.Close()
	log.Info().Int("port", port).Msg("[sip] listening")
	buf := make([]byte, 4096)
	for {
		n, ra, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		msg := string(buf[:n])
		switch {
		case strings.HasPrefix(msg, "INVITE"):
			onInvite(conn, ra, msg)
		case strings.HasPrefix(msg, "ACK"):
			// ACK confirms 200 OK. No response needed.
		case strings.HasPrefix(msg, "BYE"):
			onBye(conn, ra, msg)
		case strings.HasPrefix(msg, "CANCEL"):
			onCancel(conn, ra, msg)
		case strings.HasPrefix(msg, "OPTIONS"):
			onOptions(conn, ra, msg)
		case strings.HasPrefix(msg, "REGISTER"):
			onRegister(conn, ra, msg)
		}
	}
}

func onInvite(conn *net.UDPConn, ra *net.UDPAddr, msg string) {
	log := app.GetLogger("sip")
	callID := getHdr(msg, "Call-ID")
	if callID == "" {
		callID = randHex(8)
	}

	stream := streams.Get(cfg.Stream)
	if stream == nil {
		log.Error().Str("stream", cfg.Stream).Msg("[sip] stream not found")
		reject(conn, ra, msg, callID, 480, "Stream Not Found")
		return
	}

	// Get the camera's native codec for backchannel audio
	cameraCodec := stream.CameraCodec()

	offerSDP := extractSDP(msg)
	if offerSDP == nil {
		log.Error().Msg("[sip] failed to parse SDP offer")
		reject(conn, ra, msg, callID, 400, "Bad SDP")
		return
	}

	remoteRTP := sdpRemoteAddr(offerSDP)
	if remoteRTP == nil {
		remoteRTP = &net.UDPAddr{IP: ra.IP, Port: 5000}
	}

	// Check if caller supports PCMA or PCMU (what we use for the RTP side)
	// We prefer PCMA for maximum compatibility
	var remoteCodec *core.Codec
	if sdpHasPCMA(offerSDP) {
		remoteCodec = &core.Codec{Name: core.CodecPCMA, ClockRate: 8000, PayloadType: 8}
	} else if sdpHasPCMU(offerSDP) {
		remoteCodec = &core.Codec{Name: core.CodecPCMU, ClockRate: 8000, PayloadType: 0}
	} else {
		log.Warn().Msg("[sip] caller doesn't support PCMA or PCMU")
		reject(conn, ra, msg, callID, 488, "Codec Mismatch - Need PCMA or PCMU")
		return
	}

	// If camera codec is not set yet (stream not started), use remote codec as placeholder
	// The actual transcoding will be set up when AddConsumer is called
	if cameraCodec == nil {
		cameraCodec = remoteCodec
	}

	log.Info().Str("call_id", callID).Str("rtp", remoteRTP.String()).
		Str("remote_codec", remoteCodec.Name).Str("camera_codec", cameraCodec.Name).
		Msg("[sip] INVITE")

	localPort := allocPort()
	localIP := localAddr(ra)
	tag := randHex(4)
	sdp := buildSDPAnswer(localIP, localPort, remoteCodec)
	resp := mkResponse(msg, callID, tag, sdp, localIP, localPort)
	if _, err := conn.WriteToUDP([]byte(resp), ra); err != nil {
		log.Error().Err(err).Msg("[sip] send 200 failed")
		return
	}

	// Create RTP endpoint with remote codec (transcoding set up when track connects)
	rtpEp, err := rtp.NewRTP(remoteRTP.String(), localPort, remoteCodec)
	if err != nil {
		log.Error().Err(err).Msg("[sip] RTP create failed")
		return
	}

	sessionsMu.Lock()
	sessions[callID] = &session{
		callID: callID, tag: tag, rtp: rtpEp, stream: stream,
		createdAt: time.Now(), localIP: localIP, localPort: localPort,
	}
	sessionsMu.Unlock()

	if err := stream.AddConsumer(rtpEp); err != nil {
		log.Error().Err(err).Msg("[sip] AddConsumer failed")
		sessionsMu.Lock()
		delete(sessions, callID)
		sessionsMu.Unlock()
		rtpEp.Stop()
		return
	}

	log.Info().Str("call_id", callID).Int("port", localPort).
		Str("remote_codec", remoteCodec.Name).Str("camera_codec", cameraCodec.Name).
		Msg("[sip] answered")
}

func onBye(conn *net.UDPConn, ra *net.UDPAddr, msg string) {
	callID := getHdr(msg, "Call-ID")
	sessionsMu.Lock()
	s, ok := sessions[callID]
	if ok {
		delete(sessions, callID)
	}
	sessionsMu.Unlock()
	if ok {
		// RemoveConsumer calls Stop() internally, no need for separate s.rtp.Stop()
		if s.stream != nil {
			s.stream.RemoveConsumer(s.rtp)
		}
		// Respond with the same To tag from the original 200 OK
		conn.WriteToUDP([]byte(mkResponse(msg, callID, s.tag, "", "", 0)), ra)
	} else {
		conn.WriteToUDP([]byte(mkResponse(msg, callID, randHex(4), "", "", 0)), ra)
	}
}

func onCancel(conn *net.UDPConn, ra *net.UDPAddr, msg string) {
	callID := getHdr(msg, "Call-ID")
	sessionsMu.Lock()
	s, ok := sessions[callID]
	if ok {
		delete(sessions, callID)
	}
	sessionsMu.Unlock()
	if ok {
		log := app.GetLogger("sip")
	log.Info().Str("call_id", callID).Msg("[sip] CANCEL")
		// RemoveConsumer calls Stop() internally
		if s.stream != nil {
			s.stream.RemoveConsumer(s.rtp)
		}
		conn.WriteToUDP([]byte(mkResponse(msg, callID, s.tag, "", "", 0)), ra)
	} else {
		conn.WriteToUDP([]byte(mkResponse(msg, callID, randHex(4), "", "", 0)), ra)
	}
}

func onOptions(conn *net.UDPConn, ra *net.UDPAddr, msg string) {
	conn.WriteToUDP([]byte(mkResponse(msg, getHdr(msg, "Call-ID"), randHex(4), "", "", 0)), ra)
}

func onRegister(conn *net.UDPConn, ra *net.UDPAddr, msg string) {
	reject(conn, ra, msg, getHdr(msg, "Call-ID"), 405, "Method Not Allowed")
}

// --- SDP helpers using pion/sdp ---

func extractSDP(msg string) *sdp.SessionDescription {
	parts := strings.Split(msg, "\r\n\r\n")
	if len(parts) < 2 {
		return nil
	}
	var sd sdp.SessionDescription
	if err := sd.UnmarshalString(parts[1]); err != nil {
		return nil
	}
	return &sd
}

func sdpRemoteAddr(s *sdp.SessionDescription) *net.UDPAddr {
	if s.ConnectionInformation != nil && s.ConnectionInformation.Address != nil {
		ip := s.ConnectionInformation.Address.Address
		for _, md := range s.MediaDescriptions {
			if md.MediaName.Media == "audio" && md.MediaName.Port.Value > 0 {
				return &net.UDPAddr{IP: net.ParseIP(ip), Port: md.MediaName.Port.Value}
			}
		}
	}
	for _, md := range s.MediaDescriptions {
		if md.MediaName.Media != "audio" {
			continue
		}
		if md.ConnectionInformation != nil && md.ConnectionInformation.Address != nil {
			return &net.UDPAddr{IP: net.ParseIP(md.ConnectionInformation.Address.Address), Port: md.MediaName.Port.Value}
		}
		if md.MediaName.Port.Value > 0 {
			return &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: md.MediaName.Port.Value}
		}
	}
	return nil
}

// sdpHasPCMA checks if the SDP offer includes PCMA (payload type 8)
func sdpHasPCMA(s *sdp.SessionDescription) bool {
	for _, md := range s.MediaDescriptions {
		if md.MediaName.Media != "audio" {
			continue
		}
		for _, pt := range md.MediaName.Formats {
			if pt == "8" {
				return true
			}
		}
		for _, attr := range md.Attributes {
			if attr.Key != "rtpmap" {
				continue
			}
			if strings.HasPrefix(attr.Value, "8 ") || strings.HasPrefix(attr.Value, "PCMA") {
				return true
			}
		}
	}
	return false
}

// sdpHasPCMU checks if the SDP offer includes PCMU (payload type 0)
func sdpHasPCMU(s *sdp.SessionDescription) bool {
	for _, md := range s.MediaDescriptions {
		if md.MediaName.Media != "audio" {
			continue
		}
		for _, pt := range md.MediaName.Formats {
			if pt == "0" {
				return true
			}
		}
		for _, attr := range md.Attributes {
			if attr.Key != "rtpmap" {
				continue
			}
			if strings.HasPrefix(attr.Value, "0 ") || strings.HasPrefix(attr.Value, "PCMU") {
				return true
			}
		}
	}
	return false
}

func buildSDPAnswer(localIP string, port int, codec *core.Codec) string {
	pt := codecPT(codec.Name)
	if pt < 0 {
		pt = 96
	}
	cn := codec.Name
	switch codec.Name {
	case core.CodecPCMA:
		cn = "PCMA"
	case core.CodecPCMU:
		cn = "PCMU"
	case core.CodecG722:
		cn = "G722"
	case core.CodecOpus:
		cn = "opus"
	}
	return fmt.Sprintf(
		"v=0\r\no=- %d %d IN IP4 %s\r\ns=go2rtc\r\nc=IN IP4 %s\r\nt=0 0\r\nm=audio %d RTP/AVP %d\r\na=rtpmap:%d %s/%d\r\na=sendrecv\r\n",
		randU32(), randU32(), localIP, localIP, port, pt, pt, cn, codec.ClockRate)
}

func codecPT(name string) int {
	switch name {
	case core.CodecPCMU:
		return 0
	case core.CodecPCMA:
		return 8
	case core.CodecG722:
		return 9
	}
	return -1
}

func reject(conn *net.UDPConn, ra *net.UDPAddr, msg, callID string, code int, reason string) {
	via := getHdr(msg, "Via")
	from := getHdr(msg, "From")
	to := getHdr(msg, "To")
	cseq := getHdr(msg, "CSeq")
	resp := fmt.Sprintf("SIP/2.0 %d %s\r\nVia: %s\r\nFrom: %s\r\nTo: %s\r\nCall-ID: %s\r\nCSeq: %s\r\nContent-Length: 0\r\n\r\n",
		code, reason, via, from, to, callID, cseq)
	conn.WriteToUDP([]byte(resp), ra)
}

// getHdr extracts a SIP header value. Handles "Name: Value", "Name :Value", etc.
func getHdr(msg, name string) string {
	prefix := strings.ToLower(name + ":")
	for _, line := range strings.Split(msg, "\r\n") {
		// Trim leading spaces, then check prefix
		lower := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(lower, prefix) {
			// Extract everything after the colon
			idx := strings.Index(line, ":")
			if idx >= 0 {
				return strings.TrimSpace(line[idx+1:])
			}
		}
	}
	return ""
}

// mkResponse builds a SIP response. When sdp is non-empty, includes Content-Type and Content-Length.
// When localIP/localPort are provided, includes Contact header.
func mkResponse(req, callID, tag, sdp string, localIP string, localPort int) string {
	via := getHdr(req, "Via")
	from := getHdr(req, "From")
	to := getHdr(req, "To")
	cseq := getHdr(req, "CSeq")

	var sb strings.Builder
	fmt.Fprintf(&sb, "SIP/2.0 200 OK\r\n")
	fmt.Fprintf(&sb, "Via: %s\r\n", via)
	fmt.Fprintf(&sb, "From: %s\r\n", from)
	fmt.Fprintf(&sb, "To: %s;tag=%s\r\n", to, tag)
	fmt.Fprintf(&sb, "Call-ID: %s\r\n", callID)
	fmt.Fprintf(&sb, "CSeq: %s\r\n", cseq)

	if localIP != "" && localPort > 0 {
		fmt.Fprintf(&sb, "Contact: <sip:go2rtc@%s:%d>\r\n", localIP, cfg.Port)
	}

	if sdp != "" {
		fmt.Fprintf(&sb, "Content-Type: application/sdp\r\n")
		fmt.Fprintf(&sb, "Content-Length: %d\r\n\r\n", len(sdp))
		sb.WriteString(sdp)
	} else {
		sb.WriteString("Content-Length: 0\r\n\r\n")
	}

	return sb.String()
}

func localAddr(ra *net.UDPAddr) string {
	conn, err := net.DialUDP("udp", nil, ra)
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	h, _, _ := net.SplitHostPort(conn.LocalAddr().String())
	return h
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func randU32() uint32 {
	b := make([]byte, 4)
	rand.Read(b)
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
