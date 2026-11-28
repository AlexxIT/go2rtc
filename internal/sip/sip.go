package sip

import (
	"crypto/rand"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/rtp"
	"github.com/pion/sdp/v3"
)

// ConsumerConfig defines a single SIP auto-answer consumer instance.
type ConsumerConfig struct {
	Port   int    `yaml:"port"`
	Stream string `yaml:"stream"`
}

// SIPConfig holds global SIP settings shared across all consumers.
type SIPConfig struct {
	RTPPortRange string           `yaml:"rtp_port_range"`
	HostIP       string           `yaml:"host_ip"`
	Consumers    []ConsumerConfig `yaml:"consumers"`
}

// Global RTP port pool shared across all SIP consumers.
var (
	poolMu   sync.Mutex
	poolNext int
	poolMin  int
	poolMax  int
)

func initPool(portRange string) error {
	min, max, ok := parsePortRange(portRange)
	if !ok {
		return fmt.Errorf("invalid rtp_port_range: %q", portRange)
	}
	if max-min < 4 {
		return fmt.Errorf("rtp_port_range too small (need at least 4 ports, got %d)", max-min+1)
	}
	poolMu.Lock()
	defer poolMu.Unlock()
	poolMin = min
	poolMax = max
	poolNext = min
	return nil
}

func allocatePort() (int, error) {
	poolMu.Lock()
	defer poolMu.Unlock()
	if poolNext > poolMax {
		return 0, fmt.Errorf("rtp port pool exhausted (range %d-%d)", poolMin, poolMax)
	}
	port := poolNext
	poolNext += 2
	return port, nil
}

func parsePortRange(s string) (min, max int, ok bool) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	min, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	max, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || min >= max || min < 1024 || max > 65535 {
		return 0, 0, false
	}
	return min, max, true
}

// advertiseIP returns the IP address to advertise in SDP.
func advertiseIP(hostIP string, ra *net.UDPAddr) string {
	if hostIP != "" {
		return hostIP
	}
	return localAddr(ra)
}

// consumer is a running SIP auto-answer instance listening on its own port.
type consumer struct {
	cfg        *ConsumerConfig
	hostIP     string
	sessions   map[string]*session
	sessionsMu sync.Mutex
}

type session struct {
	callID       string
	tag          string
	rtp          *rtp.RTP
	stream       *streams.Stream
	lastActivity time.Time
	localIP      string
	localPort    int
}

func Init() {
	var cfg struct {
		SIP SIPConfig `yaml:"sip"`
	}
	app.LoadConfig(&cfg)

	rtpPortRange := cfg.SIP.RTPPortRange
	if rtpPortRange == "" {
		rtpPortRange = "31000-31100"
	}
	if err := initPool(rtpPortRange); err != nil {
		log := app.GetLogger("sip")
		log.Error().Err(err).Msg("[sip] port pool init failed")
	}

	for i := range cfg.SIP.Consumers {
		cc := &cfg.SIP.Consumers[i]
		if cc.Port == 0 {
			log := app.GetLogger("sip")
			log.Warn().Msg("[sip] consumer missing port, skipping")
			continue
		}
		if cc.Stream == "" {
			log := app.GetLogger("sip")
			log.Warn().Int("port", cc.Port).Msg("[sip] consumer has no stream, skipping")
			continue
		}
		c := &consumer{
			cfg:      cc,
			hostIP:   cfg.SIP.HostIP,
			sessions: map[string]*session{},
		}
		go c.run()
		_ = i
	}
}

func (c *consumer) run() {
	log := app.GetLogger("sip")
	log.Info().Int("port", c.cfg.Port).Str("stream", c.cfg.Stream).Msg("[sip] starting consumer")

	go c.listen(c.cfg.Port)
	go c.cleanupLoop()
}

// cleanupLoop reaps sessions with no RTP/RTCP activity for over 1 minute.
func (c *consumer) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		c.sessionsMu.Lock()
		now := time.Now()
		for id, s := range c.sessions {
			if now.Sub(s.lastActivity) > time.Minute {
				log := app.GetLogger("sip")
				log.Info().Str("call_id", id).Int("port", c.cfg.Port).Msg("[sip] reaping silent session")
				if s.stream != nil && s.rtp != nil {
					s.stream.RemoveConsumer(s.rtp)
				}
				delete(c.sessions, id)
			}
		}
		c.sessionsMu.Unlock()
	}
}

func (c *consumer) listen(port int) {
	log := app.GetLogger("sip")
	addr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Error().Err(err).Int("port", port).Msg("[sip] listen failed")
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
			c.onInvite(conn, ra, msg)
		case strings.HasPrefix(msg, "ACK"):
			// ACK confirms 200 OK. No response needed.
		case strings.HasPrefix(msg, "BYE"):
			c.onBye(conn, ra, msg)
		case strings.HasPrefix(msg, "CANCEL"):
			c.onCancel(conn, ra, msg)
		case strings.HasPrefix(msg, "OPTIONS"):
			c.onOptions(conn, ra, msg)
		case strings.HasPrefix(msg, "REGISTER"):
			c.onRegister(conn, ra, msg)
		default:
			c.updateActivityForIP(ra.IP.String())
		}
	}
}

// updateActivityForIP refreshes lastActivity for any session whose RTP endpoint
// has a matching remote IP. This catches packets arriving on the SIP port.
func (c *consumer) updateActivityForIP(ip string) {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()
	for _, s := range c.sessions {
		if s.rtp != nil && s.rtp.RemoteIP() == ip {
			s.lastActivity = time.Now()
			return
		}
	}
}

func (c *consumer) onInvite(conn *net.UDPConn, ra *net.UDPAddr, msg string) {
	log := app.GetLogger("sip")
	callID := getHdr(msg, "Call-ID")
	if callID == "" {
		callID = randHex(8)
	}

	stream := streams.Get(c.cfg.Stream)
	if stream == nil {
		log.Error().Str("stream", c.cfg.Stream).Int("port", c.cfg.Port).Msg("[sip] stream not found")
		reject(conn, ra, msg, callID, 480, "Stream Not Found")
		return
	}

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
	log.Info().Str("call_id", callID).
		Str("remote_rtp_from_sdp", remoteRTP.String()).
		Str("sip_source", ra.String()).
		Str("host_ip_override", c.hostIP).
		Msg("[sip] invite received")

	// Dial all producers so their medias are available for discovery.
	for _, prod := range stream.Producers() {
		if prod != nil {
			_ = prod.Dial()
		}
	}

	// Discover audio codecs from stream producers.
	var audioRecvonly []*core.Codec // camera sends this (main audio)
	var audioSendonly []*core.Codec // camera expects this (backchannel)

	for _, prod := range stream.Producers() {
		if prod == nil {
			continue
		}
		for _, media := range prod.GetMedias() {
			if media.Kind != core.KindAudio {
				continue
			}
			for _, codec := range media.Codecs {
				if codec.Name == core.CodecAny || codec.Name == core.CodecAll {
					continue
				}
				switch media.Direction {
				case core.DirectionRecvonly:
					audioRecvonly = append(audioRecvonly, codec)
				case core.DirectionSendonly:
					audioSendonly = append(audioSendonly, codec)
				case core.DirectionSendRecv:
					audioRecvonly = append(audioRecvonly, codec)
					audioSendonly = append(audioSendonly, codec)
				}
			}
		}
	}

	// Reject if the camera has no audio at all — no point answering.
	if len(audioRecvonly) == 0 && len(audioSendonly) == 0 {
		log.Warn().Str("call_id", callID).Msg("[sip] no audio producers on stream")
		reject(conn, ra, msg, callID, 488, "No Compatible Audio")
		return
	}

	// Negotiate the best audio codec that both camera and caller support.
	commonCodec := negotiateAudio(audioRecvonly, offerSDP)

	port, err := allocatePort()
	if err != nil {
		log.Error().Err(err).Msg("[sip] no free RTP port")
		reject(conn, ra, msg, callID, 503, "No Free Ports")
		return
	}

	localIP := advertiseIP(c.hostIP, ra)

	// Build the codec list for the SDP answer.
	// If there's main audio (recvonly from camera), use the matched codec.
	// Otherwise (speaker-only / sendonly camera), use backchannel codecs.
	// Always answer sendrecv so the caller's RTP/RTCP keeps the session alive.
	var sdpCodecs []*core.Codec
	direction := core.DirectionSendRecv

	if commonCodec != nil {
		sdpCodecs = append(sdpCodecs, commonCodec)
	}

	// Add backchannel codecs that differ from the common codec.
	for _, bc := range audioSendonly {
		if sipCodecPriority(bc.Name) == 0 {
			continue
		}
		already := false
		for _, c := range sdpCodecs {
			if c.Name == bc.Name {
				already = true
				break
			}
		}
		if !already {
			c := &core.Codec{
				Name:        bc.Name,
				ClockRate:   codecClockRate(bc.Name),
				Channels:    codecChannels(bc.Name),
				PayloadType: codecPT(bc.Name),
			}
			if bc.FmtpLine != "" {
				c.FmtpLine = bc.FmtpLine
			}
			sdpCodecs = append(sdpCodecs, c)
		}
	}

	// If we still have no codecs, there's nothing useful to offer.
	// Either the caller doesn't support the camera's audio codec, or the
	// camera's backchannel codec isn't a known SIP codec. Reject rather
	// than answer with a dead session.
	if len(sdpCodecs) == 0 {
		log.Warn().Str("call_id", callID).Msg("[sip] no compatible audio codec with caller")
		reject(conn, ra, msg, callID, 488, "No Compatible Audio")
		return
	}

	sdpAnswer := buildSDPAnswer(localIP, port, sdpCodecs)
	tag := randHex(4)
	resp := mkResponse(msg, callID, tag, sdpAnswer, localIP, port, c.cfg.Port)
	if _, err := conn.WriteToUDP([]byte(resp), ra); err != nil {
		log.Error().Err(err).Msg("[sip] send 200 failed")
		return
	}

	// Create the RTP endpoint.
	rtpEp, err := rtp.NewRTP(remoteRTP.String(), port)
	if err != nil {
		log.Error().Err(err).Msg("[sip] RTP create failed")
		return
	}

	// Register session BEFORE adding to stream so OnActivity is wired up
	// immediately and no race with the cleanup loop.
	c.sessionsMu.Lock()
	c.sessions[callID] = &session{
		callID:       callID,
		tag:          tag,
		rtp:          rtpEp,
		stream:       stream,
		lastActivity: time.Now(),
		localIP:      localIP,
		localPort:    port,
	}
	// Wire OnActivity while holding the lock so the cleanup loop
	// cannot observe a session without its callback.
	rtpEp.OnActivity = func() {
		c.sessionsMu.Lock()
		if s, ok := c.sessions[callID]; ok {
			s.lastActivity = time.Now()
		}
		c.sessionsMu.Unlock()
	}
	c.sessionsMu.Unlock()

	// Add the RTP endpoint to the stream. This wires the main audio path
	// (camera→caller) and/or the backchannel (caller→camera).
	if direction != core.DirectionRecvonly {
		if err := stream.AddConsumer(rtpEp); err != nil {
			log.Error().Err(err).Msg("[sip] AddConsumer failed")
			c.sessionsMu.Lock()
			delete(c.sessions, callID)
			c.sessionsMu.Unlock()
			rtpEp.Stop()
			return
		}
	}

	codecName := ""
	if commonCodec != nil {
		codecName = commonCodec.Name
	} else if len(audioSendonly) > 0 {
		codecName = audioSendonly[0].Name
	}
	log.Info().Str("call_id", callID).Int("port", port).
		Str("codec", codecName).
		Str("direction", direction).
		Str("send_rtp_to", remoteRTP.String()).
		Str("sdp_advertise", fmt.Sprintf("%s:%d", localIP, port)).
		Msg("[sip] answered")
}

func (c *consumer) onBye(conn *net.UDPConn, ra *net.UDPAddr, msg string) {
	callID := getHdr(msg, "Call-ID")
	c.sessionsMu.Lock()
	s, ok := c.sessions[callID]
	if ok {
		delete(c.sessions, callID)
	}
	c.sessionsMu.Unlock()
	if ok {
		log := app.GetLogger("sip")
		log.Info().Str("call_id", callID).Msg("[sip] BYE")
		if s.stream != nil && s.rtp != nil {
			s.stream.RemoveConsumer(s.rtp)
		}
		conn.WriteToUDP([]byte(mkResponse(msg, callID, s.tag, "", "", 0, 0)), ra)
	} else {
		conn.WriteToUDP([]byte(mkResponse(msg, callID, randHex(4), "", "", 0, 0)), ra)
	}
}

func (c *consumer) onCancel(conn *net.UDPConn, ra *net.UDPAddr, msg string) {
	callID := getHdr(msg, "Call-ID")
	c.sessionsMu.Lock()
	s, ok := c.sessions[callID]
	if ok {
		delete(c.sessions, callID)
	}
	c.sessionsMu.Unlock()
	if ok {
		log := app.GetLogger("sip")
		log.Info().Str("call_id", callID).Msg("[sip] CANCEL")
		if s.stream != nil && s.rtp != nil {
			s.stream.RemoveConsumer(s.rtp)
		}
		conn.WriteToUDP([]byte(mkResponse(msg, callID, s.tag, "", "", 0, 0)), ra)
	} else {
		conn.WriteToUDP([]byte(mkResponse(msg, callID, randHex(4), "", "", 0, 0)), ra)
	}
}

func (c *consumer) onOptions(conn *net.UDPConn, ra *net.UDPAddr, msg string) {
	conn.WriteToUDP([]byte(mkResponse(msg, getHdr(msg, "Call-ID"), randHex(4), "", "", 0, 0)), ra)
}

func (c *consumer) onRegister(conn *net.UDPConn, ra *net.UDPAddr, msg string) {
	reject(conn, ra, msg, getHdr(msg, "Call-ID"), 405, "Method Not Allowed")
}

// --- SDP helpers ---

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

// --- Codec helpers ---

// sipCodecPriority ranks audio codecs for negotiation preference.
func sipCodecPriority(name string) int {
	switch name {
	case core.CodecOpus:
		return 4
	case core.CodecG722:
		return 3
	case core.CodecPCMA:
		return 2
	case core.CodecPCMU:
		return 1
	default:
		return 0
	}
}

func codecClockRate(name string) uint32 {
	switch name {
	case core.CodecOpus:
		return 48000
	case core.CodecG722, core.CodecPCMA, core.CodecPCMU:
		return 8000
	}
	return 0
}

func codecChannels(name string) uint8 {
	if name == core.CodecOpus {
		return 2
	}
	return 0
}

func codecPT(name string) byte {
	switch name {
	case core.CodecPCMU:
		return 0
	case core.CodecPCMA:
		return 8
	case core.CodecG722:
		return 9
	case core.CodecOpus:
		return 111
	}
	return 96
}

func codecSDPName(name string) string {
	switch name {
	case core.CodecPCMA:
		return "PCMA"
	case core.CodecPCMU:
		return "PCMU"
	case core.CodecG722:
		return "G722"
	case core.CodecOpus:
		return "opus"
	}
	return name
}

// callerHasAudioCodec checks if the SDP offer includes the named audio codec.
func callerHasAudioCodec(s *sdp.SessionDescription, name string) bool {
	pt := fmt.Sprintf("%d", codecPT(name))
	sdpName := codecSDPName(name)
	for _, md := range s.MediaDescriptions {
		if md.MediaName.Media != "audio" {
			continue
		}
		for _, f := range md.MediaName.Formats {
			if f == pt {
				return true
			}
		}
		for _, attr := range md.Attributes {
			if attr.Key != "rtpmap" {
				continue
			}
			v := strings.ToLower(attr.Value)
			if strings.HasPrefix(v, pt+" ") || strings.HasPrefix(v, strings.ToLower(sdpName)) {
				return true
			}
		}
	}
	return false
}

// negotiateAudio finds the best audio codec supported by both the caller (SDP
// offer) and the stream producers (recvonly direction). Returns nil if no
// common codec exists.
func negotiateAudio(prodCodecs []*core.Codec, offer *sdp.SessionDescription) *core.Codec {
	preferred := []string{core.CodecOpus, core.CodecG722, core.CodecPCMA, core.CodecPCMU}
	for _, name := range preferred {
		if !callerHasAudioCodec(offer, name) {
			continue
		}
		for _, pc := range prodCodecs {
			if pc.Name != name {
				continue
			}
			cr := codecClockRate(name)
			if pc.ClockRate != 0 && pc.ClockRate != cr {
				continue
			}
			c := &core.Codec{
				Name:        name,
				ClockRate:   cr,
				Channels:    codecChannels(name),
				PayloadType: codecPT(name),
			}
			// Carry forward the producer's fmtp so Opus params
			// (useinbandfec, stereo, maxplaybackrate, etc.) reach the caller.
			if pc.FmtpLine != "" {
				c.FmtpLine = pc.FmtpLine
			}
			return c
		}
	}
	return nil
}



func buildSDPAnswer(localIP string, port int, codecs []*core.Codec) string {
	sdp := fmt.Sprintf(
		"v=0\r\no=- %d %d IN IP4 %s\r\ns=go2rtc\r\nc=IN IP4 %s\r\nt=0 0\r\n",
		randU32(), randU32(), localIP, localIP)

	// go2sip: standard SIP always uses ONE sendrecv m=audio line. List every
	// negotiated payload type on a single line and mark it sendrecv, even if one
	// direction is missing, so the caller's RTP/RTCP still keeps the session
	// alive. No multi-line recvonly fan-out.
	seen := make(map[uint8]bool)
	pts := make([]uint8, 0, len(codecs))
	for _, c := range codecs {
		if seen[c.PayloadType] {
			continue
		}
		seen[c.PayloadType] = true
		pts = append(pts, c.PayloadType)
	}

	ptStr := ""
	for i, pt := range pts {
		if i > 0 {
			ptStr += " "
		}
		ptStr += strconv.Itoa(int(pt))
	}
	sdp += fmt.Sprintf("m=audio %d RTP/AVP %s\r\n", port, ptStr)

	for i := range pts {
		seen[pts[i]] = false // emit rtpmap/fmtp once per PT
	}
	for _, c := range codecs {
		if seen[c.PayloadType] {
			continue
		}
		seen[c.PayloadType] = true
		pt := c.PayloadType
		sdp += fmt.Sprintf("a=rtpmap:%d %s/%d",
			pt, codecSDPName(c.Name), c.ClockRate)
		if c.Channels > 0 {
			sdp += fmt.Sprintf("/%d", c.Channels)
		}
		sdp += "\r\n"
		if c.FmtpLine != "" {
			sdp += fmt.Sprintf("a=fmtp:%d %s\r\n", pt, c.FmtpLine)
		}
	}

	sdp += fmt.Sprintf("a=%s\r\n", core.DirectionSendRecv)

	// Advertise RTCP port (RTP port + 1) for keepalive.
	if port+1 <= 65535 {
		sdp += fmt.Sprintf("a=rtcp:%d\r\n", port+1)
	}

	return sdp
}

// allSameCodec returns true when all codecs in the list share the same name.
func allSameCodec(codecs []*core.Codec) bool {
	if len(codecs) <= 1 {
		return true
	}
	name := codecs[0].Name
	for _, c := range codecs[1:] {
		if c.Name != name {
			return false
		}
	}
	return true
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

// getHdr extracts a SIP header value.
func getHdr(msg, name string) string {
	prefix := strings.ToLower(name + ":")
	for _, line := range strings.Split(msg, "\r\n") {
		lower := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(lower, prefix) {
			idx := strings.Index(line, ":")
			if idx >= 0 {
				return strings.TrimSpace(line[idx+1:])
			}
		}
	}
	return ""
}

// mkResponse builds a SIP response.
func mkResponse(req, callID, tag, sdp string, localIP string, localPort, sipPort int) string {
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
		fmt.Fprintf(&sb, "Contact: <sip:go2rtc@%s:%d>\r\n", localIP, sipPort)
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
