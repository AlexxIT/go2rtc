package dtls

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/tutk"
	piondtls "github.com/pion/dtls/v3"
)

func configuredTestConn(options Options, authKey string) *DTLSConn {
	options, psk := normalizeOptions(options, "synthetic-enr", false)
	return &DTLSConn{
		uid:                  "synthetic-uid",
		authKey:              authKey,
		enr:                  "synthetic-enr",
		psk:                  psk,
		pskIdentity:          PSKIdentity(options.AVUsername),
		outerProtocolVersion: options.OuterProtocolVersion,
		sdkVersion:           options.SDKVersion,
		avUsername:           options.AVUsername,
		avPassword:           options.AVPassword,
		avLoginSecurityMode:  options.AVLoginSecurityMode,
		cipherSuites:         options.CipherSuites,
		sid:                  []byte("sid12345"),
	}
}

func TestConfiguredDiscoveryRequest(t *testing.T) {
	c := configuredTestConn(Options{
		OuterProtocolVersion: 29,
		SDKVersion:           [4]byte{0x03, 0x06, 0x03, 0x04},
		AVUsername:           "synthetic-user",
		AVPassword:           "synthetic-password",
		AVLoginSecurityMode:  2,
	}, "")

	packet := c.msgDisco(2)
	if len(packet) != 88 {
		t.Fatalf("discovery packet length = %d, want 88", len(packet))
	}
	if !bytes.Equal(packet[:4], []byte{0x04, 0x02, 0x1d, 0x02}) {
		t.Fatal("discovery marker, protocol version, or mode mismatch")
	}
	if binary.LittleEndian.Uint16(packet[8:]) != cmdDiscoReq {
		t.Fatal("discovery command mismatch")
	}
	if binary.LittleEndian.Uint16(packet[10:]) != 0x0021 {
		t.Fatal("discovery flags mismatch")
	}
	if !bytes.Equal(packet[52:56], []byte{0x03, 0x06, 0x03, 0x04}) {
		t.Fatal("discovery SDK version mismatch")
	}
	if packet[64] != 2 {
		t.Fatalf("discovery stage = %d, want 2", packet[64])
	}
	if !bytes.Equal(packet[74:], make([]byte, 14)) {
		t.Fatal("empty AuthKey field is not zero-filled")
	}
}

func TestDiscoveryAuthKeyField(t *testing.T) {
	c := configuredTestConn(Options{
		OuterProtocolVersion: 29,
		SDKVersion:           [4]byte{0x03, 0x06, 0x03, 0x04},
	}, "synthetic-auth")

	stageOne := c.msgDisco(1)
	if !bytes.Equal(stageOne[74:88], []byte("synthetic-auth")) {
		t.Fatal("non-empty AuthKey was not written to stage-one discovery")
	}

	stageTwo := c.msgDisco(2)
	if !bytes.Equal(stageTwo[74:], make([]byte, 14)) {
		t.Fatal("stage-two discovery unexpectedly contains AuthKey")
	}
}

func TestConfiguredOuterProtocolVersion(t *testing.T) {
	c := configuredTestConn(Options{OuterProtocolVersion: 29}, "")

	session := c.msgSession()
	if len(session) != 52 {
		t.Fatalf("session packet length = %d, want 52", len(session))
	}
	if session[2] != 29 || binary.LittleEndian.Uint16(session[8:]) != cmdSessionReq || binary.LittleEndian.Uint16(session[10:]) != 0x0033 {
		t.Fatal("session packet does not use the configured outer protocol fields")
	}

	data := c.msgTxData([]byte{0x01, 0x02}, 0)
	if data[2] != 29 || binary.LittleEndian.Uint16(data[8:]) != cmdDataTX || binary.LittleEndian.Uint16(data[10:]) != 0x0021 {
		t.Fatal("data packet does not use the configured outer protocol fields")
	}

	keepalive := c.msgKeepalive(nil)
	if keepalive[2] != 29 || binary.LittleEndian.Uint16(keepalive[8:]) != cmdKeepaliveReq || binary.LittleEndian.Uint16(keepalive[10:]) != 0x0021 {
		t.Fatal("keepalive packet does not use the configured outer protocol fields")
	}
}

func TestConfiguredAVLogin(t *testing.T) {
	c := configuredTestConn(Options{
		OuterProtocolVersion: 29,
		AVUsername:           "synthetic-user",
		AVPassword:           "synthetic-password",
		AVLoginSecurityMode:  2,
	}, "")

	packet := c.msgAVLogin(magicAVLogin1, 570, 0x0001, []byte{1, 2, 3, 4})
	if len(packet) != 570 {
		t.Fatalf("AV login packet length = %d, want 570", len(packet))
	}
	if binary.LittleEndian.Uint16(packet) != magicAVLogin1 ||
		binary.LittleEndian.Uint16(packet[16:]) != 546 ||
		binary.LittleEndian.Uint16(packet[18:]) != 0x0001 ||
		!bytes.Equal(packet[20:24], []byte{1, 2, 3, 4}) {
		t.Fatal("AV login header fields mismatch")
	}
	if binary.LittleEndian.Uint16(packet[2:]) != avProtocolVersion {
		t.Fatal("AV login uses the wrong inner protocol version")
	}
	if !bytes.Equal(packet[24:280], paddedTestField("synthetic-user", 256)) {
		t.Fatal("AV login username field mismatch")
	}
	if !bytes.Equal(packet[280:536], paddedTestField("synthetic-password", 256)) {
		t.Fatal("AV login password field mismatch")
	}
	if binary.LittleEndian.Uint32(packet[540:]) != 2 {
		t.Fatal("AV login security mode mismatch")
	}

	packet2 := c.msgAVLogin(magicAVLogin2, 572, 0, []byte{1, 2, 3, 4})
	if len(packet2) != 572 || binary.LittleEndian.Uint16(packet2) != magicAVLogin2 {
		t.Fatal("second AV login request shape mismatch")
	}
}

func TestPSKIdentityAndPassword(t *testing.T) {
	const password = "synthetic-password"

	c := configuredTestConn(Options{
		AVUsername: "synthetic-user",
		AVPassword: password,
	}, "")
	if got := c.pskIdentity; got != "AUTHPWD_synthetic-user" {
		t.Fatal("configured PSK identity mismatch")
	}
	if !bytes.Equal(c.psk, DerivePSK(password)) {
		t.Fatal("configured AV password was not used to derive the DTLS PSK")
	}

	config := buildDTLSConfigWithIdentity(c.psk, c.pskIdentity, false)
	if !bytes.Equal(config.PSKIdentityHint, []byte("AUTHPWD_synthetic-user")) {
		t.Fatal("DTLS config PSK identity mismatch")
	}
	psk, err := config.PSK(nil)
	if err != nil || !bytes.Equal(psk, DerivePSK(password)) {
		t.Fatal("DTLS config PSK mismatch")
	}
}

func TestConfiguredCipherSuites(t *testing.T) {
	c := configuredTestConn(Options{
		CipherSuites: []piondtls.CipherSuiteID{
			piondtls.TLS_PSK_WITH_AES_128_CBC_SHA256,
		},
	}, "")
	if len(c.cipherSuites) != 1 ||
		c.cipherSuites[0] != piondtls.TLS_PSK_WITH_AES_128_CBC_SHA256 {
		t.Fatal("configured DTLS cipher suite mismatch")
	}
}

func TestLegacyDefaultsRemainCompatible(t *testing.T) {
	options, psk := normalizeOptions(Options{}, "synthetic-enr", true)
	if options.OuterProtocolVersion != outerProtocolVersion {
		t.Fatal("legacy outer protocol version changed")
	}
	if options.SDKVersion != defaultSDKVersion {
		t.Fatal("legacy SDK version changed")
	}
	if options.AVLoginSecurityMode != 4 {
		t.Fatal("legacy AV login security mode changed")
	}
	if !bytes.Equal(psk, DerivePSK("synthetic-enr")) {
		t.Fatal("legacy DTLS PSK derivation changed")
	}

	config := buildDTLSConfig(psk, false)
	if !bytes.Equal(config.PSKIdentityHint, []byte(PSKIdentity("admin"))) {
		t.Fatal("legacy DTLS PSK identity changed")
	}
}

func TestAVTwoWaySpeakerControlsUseClientDTLS(t *testing.T) {
	writer := &captureWriter{}
	c := configuredTestConn(Options{}, "")
	c.clientWriter = writer
	c.hasTwoWayStreaming = true

	const channel = uint32(1)
	if err := c.AVTwoWayStart(channel); err != nil {
		t.Fatalf("AVTwoWayStart failed: %v", err)
	}
	if c.serverConn != nil {
		t.Fatal("AVTwoWayStart unexpectedly initialized the separate server connection")
	}
	assertSpeakerControl(t, writer.data[0], speakerStartControl, channel, 0)

	if err := c.AVTwoWayStop(channel); err != nil {
		t.Fatalf("AVTwoWayStop failed: %v", err)
	}
	assertSpeakerControl(t, writer.data[1], speakerStopControl, channel, 1)
	if c.twoWayStarted {
		t.Fatal("two-way state remained active after stop")
	}
	if c.twoWayChannel != 0 || c.twoWayAudioSeq != 0 || c.twoWayAudioFrameNo != 0 {
		t.Fatal("two-way state was not reset after stop")
	}

	if err := c.AVTwoWayStop(channel); err != nil {
		t.Fatalf("second AVTwoWayStop failed: %v", err)
	}
	if len(writer.data) != 2 {
		t.Fatalf("idempotent stop wrote %d frames, want 2", len(writer.data))
	}
}

func TestAVTwoWayStartRequiresNegotiatedSupport(t *testing.T) {
	writer := &captureWriter{}
	c := configuredTestConn(Options{}, "")
	c.clientWriter = writer

	if err := c.AVTwoWayStart(1); err == nil {
		t.Fatal("AVTwoWayStart succeeded without negotiated two-way support")
	}
	if len(writer.data) != 0 {
		t.Fatal("unsupported AVTwoWayStart wrote a control frame")
	}
}

func TestAudioLogsDoNotContainPayload(t *testing.T) {
	const privatePayloadMarker = "private-audio-payload"
	for _, direction := range []string{"SERVER TX", "CLIENT TX"} {
		got := formatAudioLog(direction, 692, tutk.CodecPCMU, 40000)
		if strings.Contains(got, privatePayloadMarker) {
			t.Fatalf("audio log contains payload marker: %q", got)
		}
		if !strings.Contains(got, "["+direction+"]") ||
			!strings.Contains(got, "len=692") ||
			!strings.Contains(got, "codec=0x89") ||
			!strings.Contains(got, "timestamp_us=40000") {
			t.Fatalf("audio log missing safe metadata: %q", got)
		}
	}
}

func TestAVTwoWayPCMUAudioTimestampsMatchCAM720FrameFormat(t *testing.T) {
	writer := &captureWriter{}
	c := configuredTestConn(Options{}, "")
	c.clientWriter = writer
	c.hasTwoWayStreaming = true

	payloads := make([][]byte, 3)
	for frameNo := range payloads {
		payloads[frameNo] = make([]byte, 640)
		for i := range payloads[frameNo] {
			payloads[frameNo][i] = byte(frameNo + i)
		}
	}

	if err := c.AVSendAudioDataTwoWay(tutk.CodecPCMU, payloads[0], 0, 16000, 1); err == nil {
		t.Fatal("AVSendAudioDataTwoWay succeeded before speaker start")
	}
	if len(writer.data) != 0 {
		t.Fatal("audio sent before speaker start")
	}

	if err := c.AVTwoWayStart(1); err != nil {
		t.Fatalf("AVTwoWayStart failed: %v", err)
	}
	for frameNo, payload := range payloads {
		if err := c.AVSendAudioDataTwoWay(tutk.CodecPCMU, payload, uint32(frameNo*40), 16000, 1); err != nil {
			t.Fatalf("AVSendAudioDataTwoWay frame %d failed: %v", frameNo, err)
		}
	}
	if c.serverConn != nil {
		t.Fatal("two-way audio unexpectedly used the separate server connection")
	}

	if len(writer.data) != 4 {
		t.Fatalf("captured frames = %d, want speaker start plus 3 audio frames", len(writer.data))
	}
	for i, payload := range payloads {
		frame := writer.data[i+1]
		if len(frame) != 36+640+16 {
			t.Fatalf("audio frame %d length = %d, want 692", i, len(frame))
		}
		frameNo := uint32(i + 1)
		prevFrame := uint32(i)
		wantOuterFlags := uint32(0x00100001)
		if i == 0 {
			wantOuterFlags = 1
		}
		if frame[0] != tutk.ChannelAudio || frame[1] != tutk.FrameTypeStartAlt ||
			binary.LittleEndian.Uint16(frame[2:]) != avProtocolVersion ||
			binary.LittleEndian.Uint32(frame[4:]) != frameNo ||
			binary.LittleEndian.Uint32(frame[8:]) != uint32(i*40) ||
			binary.LittleEndian.Uint32(frame[12:]) != wantOuterFlags {
			t.Fatalf("audio frame %d outer header mismatch", i)
		}
		if frame[16] != tutk.ChannelAudio || frame[17] != tutk.FrameTypeEndSingle ||
			binary.LittleEndian.Uint16(frame[18:]) != uint16(prevFrame) ||
			binary.LittleEndian.Uint16(frame[20:]) != 1 ||
			binary.LittleEndian.Uint16(frame[22:]) != 0x0010 ||
			binary.LittleEndian.Uint32(frame[24:]) != 656 ||
			binary.LittleEndian.Uint32(frame[28:]) != prevFrame ||
			binary.LittleEndian.Uint32(frame[32:]) != frameNo {
			t.Fatalf("audio frame %d inner header mismatch", i)
		}
		if !bytes.Equal(frame[36:676], payload) {
			t.Fatalf("audio frame %d payload mismatch", i)
		}

		frameInfo := frame[676:]
		if frameInfo[0] != tutk.CodecPCMU ||
			frameInfo[2] != 14 ||
			frameInfo[4] != 1 ||
			binary.LittleEndian.Uint32(frameInfo[12:]) != uint32(i*10) {
			t.Fatalf("audio frame %d frame info timestamp mismatch", i)
		}
	}
}

func TestAVTwoWayNonCAM720PCMUUsesPayloadDuration(t *testing.T) {
	writer := &captureWriter{}
	c := configuredTestConn(Options{}, "")
	c.clientWriter = writer
	c.hasTwoWayStreaming = true
	if err := c.AVTwoWayStart(1); err != nil {
		t.Fatalf("AVTwoWayStart failed: %v", err)
	}

	payload := make([]byte, 320)
	for i := 0; i < 2; i++ {
		if err := c.AVSendAudioDataTwoWay(tutk.CodecPCMU, payload, uint32(i*20), 16000, 1); err != nil {
			t.Fatalf("AVSendAudioDataTwoWay frame %d failed: %v", i, err)
		}
	}
	frame := writer.data[2]
	frameInfoTimestamp := binary.LittleEndian.Uint32(frame[36+len(payload)+12:])
	if frameInfoTimestamp != 20000 {
		t.Fatalf("frame-info timestamp = %d, want original payload duration 20000", frameInfoTimestamp)
	}
}

func TestAVTwoWayCloseClearsState(t *testing.T) {
	writer := &captureWriter{}
	c := configuredTestConn(Options{}, "")
	c.clientWriter = writer
	c.cancel = func() {}
	c.hasTwoWayStreaming = true

	if err := c.AVTwoWayStart(1); err != nil {
		t.Fatalf("AVTwoWayStart failed: %v", err)
	}
	c.twoWayAudioSeq = 4
	c.twoWayAudioFrameNo = 5

	if err := c.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if c.hasTwoWayStreaming || c.twoWayStarted || c.twoWayChannel != 0 ||
		c.twoWayAudioSeq != 0 || c.twoWayAudioFrameNo != 0 {
		t.Fatal("Close did not clear two-way state")
	}
}

func assertSpeakerControl(t *testing.T, frame []byte, control, channel uint32, seq uint16) {
	t.Helper()
	if len(frame) != 52 {
		t.Fatalf("speaker control frame length = %d, want 52", len(frame))
	}
	if binary.LittleEndian.Uint16(frame) != avProtocolVersion ||
		binary.LittleEndian.Uint16(frame[2:]) != avProtocolVersion ||
		binary.LittleEndian.Uint16(frame[16:]) != magicIOCtrl ||
		binary.LittleEndian.Uint16(frame[18:]) != seq ||
		binary.LittleEndian.Uint32(frame[24:]) != 16 {
		t.Fatal("speaker control IOCTRL header mismatch")
	}
	if binary.LittleEndian.Uint32(frame[40:]) != control ||
		binary.LittleEndian.Uint32(frame[44:]) != channel ||
		binary.LittleEndian.Uint32(frame[48:]) != 0 {
		t.Fatalf("speaker control payload = %x", frame[40:])
	}
}

type captureWriter struct {
	data [][]byte
}

func (w *captureWriter) Write(data []byte) (int, error) {
	w.data = append(w.data, append([]byte(nil), data...))
	return len(data), nil
}

func paddedTestField(value string, size int) []byte {
	field := make([]byte, size)
	copy(field, value)
	return field
}
