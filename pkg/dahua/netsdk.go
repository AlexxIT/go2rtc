package dahua

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
)

// ---------------------------------------------------------------------------
// Dahua NetSDK binary two-way talk on TCP/37777
// ---------------------------------------------------------------------------
//
// This is the NetSDK transport: the binary talk path (CLIENT_StartTalkEx) that
// Dahua's own client uses over TCP/37777. It is not the speak.* JSON-RPC family
// (trimmed firmware answers InterfaceNotFound to every speak.* method yet talks
// fine here), nor the DHIP JSON-RPC framing on TCP/5000, nor the DVRIP framing
// in pkg/dvrip (Xiongmai / XMeye hardware).
//
// The whole sequence was recovered byte-for-byte from a live capture of the
// prebuilt Talk.exe demo (SDK V3.061) driving a Dahua E4702. The
// login/handshake frames were diffed byte-for-byte against that Talk.exe
// capture; audio frames reuse the same DHAV sub-header layout (codec id from
// negotiation, length field at [6:8], etc.).
//
// Wire format: 32 byte header + body, body length at header[4:8] LE.
//
//	CONTROL CONNECTION
//	  -> A0 len=0   header[1:4]=05 00 60  header[24:32]=05 02 00 01 00 00 a1 aa
//	  <- B0         "Realm:Login to XXXX\r\nRandom:NNNNd"
//	  -> A0 len=71  header[1:4]=05 00 60  header[24:32]=05 02 00 08 00 00 a1 aa
//	                body "user&&<GEN2 32hex><GEN1 32hex>"
//	  <- B0         session id = uint32 LE at header[16:20]
//	  -> F4         Method:AddObject
//	                ParameterName:Dahua.Device.Network.ControlConnection.Passive
//	  <- F4         AddObjectResponse ... ConnectionID:<CID>
//	  -> A1         keepalive once per second, camera answers B1
//
//	SUB CHANNEL (a second TCP connection to the same port)
//	  -> F4         Method:GetParameterNames
//	                ParameterName:...ControlConnection.AckSubChannel
//	                SessionID:<SID> ConnectionID:<CID> Encrypt:0
//	  <- F4         FaultCode:OK
//	  -> A1         once
//
//	START TALK (on the control connection, after the sub channel is acked)
//	  -> F4         Method:GetParameterNames
//	                ParameterName:Dahua.Device.Network.Talk.General
//	                Channel:0 EncodeFormat:1 Depth:16 Frequency:8000
//	                State:1 ConnectionID:<CID> TalkMode:0
//
//	AUDIO (on the sub channel, both directions, command 0x1D)
//	  upstream   32 byte header + 8 byte DHAV sub-header + payload
//	             header[8]=02 [9:13]=bits [13:17]=channels [17:21]=rate
//	             sub = 00 00 01 F0 <codec> <rateIdx> <len uint16 LE>
//	  downstream 32 byte header (rest zero) + a full DHAV container,
//	             see parseDHAVAudio
//
//	STOP  -> F4 Talk.General with Depth:0 Frequency:0 State:0
//	      -> F4 Method:DeleteObject ... ConnectionID:<CID>

const (
	talkHeaderSize = 32

	// maxFrame bounds a single wire frame body. Real talk frames never exceed a
	// few hundred bytes; 64 KiB is ample and turns a corrupt length field into a
	// fast, explicit error instead of a multi-megabyte stall.
	maxFrame = 64 * 1024

	cmdLogin     = 0xA0 // client -> device, login / challenge
	cmdLoginResp = 0xB0
	cmdKeepAlive = 0xA1
	cmdKeepResp  = 0xB1
	cmdText      = 0xF4 // key:value config channel, both directions
	cmdTalkData  = 0x1D // talk audio, both directions

	// DHAV audio codec ids carried in the talk sub-header. Both were confirmed
	// on E4702 hardware: the speaker rendered the same test motif from 0x0C
	// (640 byte payload) and from 0x0E (320 byte payload). 0x0E is also what the
	// camera uses for its own microphone, so the E4702 accepts G.711 upstream.
	dhavCodecPCM16 = 0x0C
	dhavCodecG711A = 0x0E
	dhavCodecG711U = 0x0A
)

var (
	trailerChallenge = [8]byte{0x05, 0x02, 0x00, 0x01, 0x00, 0x00, 0xA1, 0xAA}
	trailerLogin     = [8]byte{0x05, 0x02, 0x00, 0x08, 0x00, 0x00, 0xA1, 0xAA}
)

// NetSDKTransport speaks the binary talk protocol directly, with no dependency
// on the JSON-RPC layer. It owns its two TCP connections.
type NetSDKTransport struct {
	addr    string
	user    string
	pass    string
	channel int
	timeout time.Duration

	// encodeFormat is the EncodeFormat value of the Talk.General request. It
	// maps to the Win64 NetSDK DH_TALK_CODING_TYPE enum (1 = DH_TALK_PCM,
	// "with-head PCM"). The official Talk demo exposes only one choice in the
	// UI ("PCM_16Bit_8000SampleRate") and hardcodes encodeType=DH_TALK_PCM,
	// dwSampleRate=8000, nAudioBit=16, so the default 1 matches the vendor
	// client.
	encodeFormat int

	// forcedCodec pins the codec offered to go2rtc, skipping the default list.
	forcedCodec string

	// nativeG711 sends G.711 straight to the camera instead of decoding it to
	// PCM16 first. On by default because the E4702 accepts G.711 upstream (0x0E
	// verified on hardware) and passthrough avoids a decode step and half the
	// bytes. Set native=0 on the URL to decode to PCM16 instead; this
	// reproduces the official Talk demo's PCM16 path (DH_TALK_PCM, 0x0C) for
	// firmware that only accepts PCM16.
	nativeG711 bool

	// decodeFrom is "" (pass through) or the codec name we must decode into
	// PCM16 inside WriteAudio. Set by Open.
	decodeFrom string

	// OnAudio, when set, receives the camera microphone payload together with
	// its Dahua codec id. This is what makes the channel full duplex.
	OnAudio func(payload []byte, codecID byte)

	// Debug mirrors the hook used by the other transports.
	Debug func(dir string, b []byte)

	mu      sync.Mutex
	ctrlMu  sync.Mutex // serialises writes to ctrl (keepAliveLoop vs Close)
	ctrl    net.Conn
	sub     net.Conn
	session uint32
	connID  string

	codecID   byte
	rateIndex byte
	rate      uint32
	frameSize int
	opened    bool
	closed    bool
	stopKeep  chan struct{}
}

// NewNetSDKTransport builds the transport. Nothing is dialled until Open.
func NewNetSDKTransport(host, user, pass string, channel int, timeout time.Duration) *NetSDKTransport {
	if !strings.Contains(host, ":") {
		host = net.JoinHostPort(host, "37777")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &NetSDKTransport{
		addr:         host,
		user:         user,
		pass:         pass,
		channel:      channel,
		timeout:      timeout,
		encodeFormat: 1,
		nativeG711:   true,
		stopKeep:     make(chan struct{}),
	}
}

// Codecs reports what we can push, best first.
//
// PCMA leads because a WebRTC browser can only offer Opus, G.711 or G722, so
// PCMA is the one entry that negotiates with no transcoding, and the E4702
// accepts G.711 upstream (0x0E verified on hardware). PCML stays in the list
// because the DHAV sub-header carries the codec per frame, so firmware that
// only accepts 0x0C still works when a caller pins it with codec=pcml.
func (t *NetSDKTransport) Codecs() []*core.Codec {
	if t.forcedCodec != "" {
		return []*core.Codec{{
			Name:        t.forcedCodec,
			ClockRate:   8000,
			Channels:    1,
			PayloadType: payloadType(t.forcedCodec),
		}}
	}
	return []*core.Codec{
		{Name: core.CodecPCMA, ClockRate: 8000, Channels: 1, PayloadType: payloadType(core.CodecPCMA)},
		{Name: core.CodecPCMU, ClockRate: 8000, Channels: 1, PayloadType: payloadType(core.CodecPCMU)},
		{Name: core.CodecPCML, ClockRate: 8000, Channels: 1, PayloadType: payloadType(core.CodecPCML)},
	}
}

func (t *NetSDKTransport) FrameSize() int { return t.frameSize }

// Open runs the full handshake and leaves the sub channel ready for audio.
func (t *NetSDKTransport) Open(codec *core.Codec) error {
	codecID, ok := dhavAudioCodecID(codec.Name)
	if !ok {
		return fmt.Errorf("dahua: unsupported talk codec %q", codec.Name)
	}

	// A WebRTC browser can only send Opus or G.711, so the negotiated codec is
	// almost always PCMA. PCM16 is the format used by the official demo path; on
	// this firmware G.711 A-law (0x0E) has also been verified to render
	// (nativeG711, the default when native=1), so both are proven working. Decode
	// G.711 here unless the user opted out.
	decodeFrom := ""
	if !t.nativeG711 && (codec.Name == core.CodecPCMA || codec.Name == core.CodecPCMU) {
		decodeFrom = codec.Name
		codecID = dhavCodecPCM16
	}

	rate := codec.ClockRate
	if rate == 0 {
		rate = 8000
	}

	// A failed Open tears itself down via Close (latches closed, closes
	// stopKeep). Reset here so a retry doesn't start its keepalive goroutine
	// on an already-closed channel - the camera would see no 0xA1 and drop the
	// session a few seconds later, masking the real failure.
	t.mu.Lock()
	if t.closed {
		t.closed = false
		t.stopKeep = make(chan struct{})
	}
	t.mu.Unlock()

	if err := t.dialAndLogin(); err != nil {
		return err
	}

	var err error
	var cid string
	if cid, err = t.addObject(); err != nil {
		t.Close()
		return err
	}
	// Publish the connection id under the lock so setTalkState's lock-protected
	// read (and the Close teardown) stay race-free with this write.
	t.mu.Lock()
	t.connID = cid
	t.mu.Unlock()

	if t.sub, err = net.DialTimeout("tcp", t.addr, t.timeout); err != nil {
		t.Close()
		return fmt.Errorf("dahua: dial sub channel: %w", err)
	}

	if err = t.ackSubChannel(); err != nil {
		t.Close()
		return err
	}

	// the official client sends one bare keepalive before the first audio
	if _, err = t.sub.Write(buildFrame(cmdKeepAlive, nil, nil)); err != nil {
		t.Close()
		return err
	}

	if err = t.setTalkState(true, rate); err != nil {
		t.Close()
		return err
	}

	// These fields are read under t.mu by WriteAudio / keepAliveLoop, and the
	// goroutines that read them only start below. The lock here keeps the
	// assignment correct even if Open is ever called from its own goroutine or
	// concurrently - it is the happens-before the original code relied on
	// implicitly.
	t.mu.Lock()
	t.codecID = codecID
	t.decodeFrom = decodeFrom
	t.rateIndex = dhavSampleRateIndex(rate)
	t.rate = rate
	t.frameSize = frameBytes(codec, 40)
	// Compressed codecs (e.g. AAC) make frameBytes return 0: forward each RTP
	// payload as-is instead of slicing at a fixed 320-byte boundary, which would
	// split ADTS frames. PCMA/PCMU/PCML keep their fixed chunk size.
	if t.frameSize <= 0 {
		t.frameSize = 0
	}
	t.opened = true
	t.mu.Unlock()

	go t.keepAliveLoop()
	go t.receiveLoop()

	return nil
}

// WriteAudio pushes one payload chunk wrapped in a 0x1D talk frame.
func (t *NetSDKTransport) WriteAudio(payload []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.opened || t.sub == nil {
		return 0, fmt.Errorf("dahua: talk channel not open")
	}

	if t.decodeFrom != "" {
		payload = g711ToPCM16(payload, t.decodeFrom)
	}

	frame := buildTalkAudio(payload, t.codecID, t.rateIndex, t.rate)

	// Bound the write so a stalled camera (TCP send buffer full, device stops
	// reading) cannot block this goroutine forever. The Write below runs while
	// t.mu is held; without a deadline a blocked Write would also deadlock
	// Stop()/Close() (which need the same mutex) and hang the WebRTC teardown.
	if err := t.sub.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return 0, err
	}
	n, err := t.sub.Write(frame)
	if err != nil {
		_ = t.sub.SetWriteDeadline(time.Time{}) // clear so later Close is clean
		return n, err
	}
	if t.Debug != nil {
		t.Debug("talk ->", frame[:min(len(frame), 48)])
	}
	return n, err
}

func (t *NetSDKTransport) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	wasOpen := t.opened
	t.opened = false
	// Capture the connection handles under the lock so the rest of teardown
	// runs on stable locals. setTalkState / sendText serialise the actual
	// writes via ctrlMu; keepAliveLoop and receiveLoop only ever read these
	// pointers (never reassign them mid-session), so once captured here they
	// stay valid for the Close calls below even after the lock is dropped.
	ctrl := t.ctrl
	sub := t.sub
	connID := t.connID
	rate := t.rate
	close(t.stopKeep)
	t.mu.Unlock()

	if wasOpen && ctrl != nil {
		_ = t.setTalkState(false, rate)
		_ = t.sendText(ctrl, []string{
			"TransactionID:9",
			"Method:DeleteObject",
			"ParameterName:Dahua.Device.Network.ControlConnection.Passive",
			"ConnectionID:" + connID,
		})
	}

	if sub != nil {
		_ = sub.Close()
	}
	if ctrl != nil {
		_ = ctrl.Close()
	}
	return nil
}

// ---------------------------------------------------------------------------
// handshake steps
// ---------------------------------------------------------------------------

// loginBackoff throttles reconnects after the camera rejected these exact
// credentials (wrong password, unknown user, account lockout, blocklist).
//
// Validated on E4702: the device enforces a PER-SESSION login lock, not a
// global account one. A refused login carries a "LockLeftTime" timer scoped to
// the failing connection; other clients (and a corrected config) keep working
// while that session is locked. The brake prevents go2rtc from re-hammering its
// own locked session and re-extending that window, and is defensive against a
// real global lockout on older firmware/NVRs.
//
// This is a LOCAL 30s brake keyed by addr|user|passFingerprint. It is NOT the
// device-side LockLeftTime lock: correcting the password changes the key and
// retries at once, no need to wait out the window.
var loginBackoff sync.Map // credentials key -> time.Time of the last refusal

const loginRetryBackoff = 30 * time.Second

func (t *NetSDKTransport) backoffKey() string {
	return t.addr + "|" + t.user + "|" + passFingerprint(t.pass)
}

// dialAndLogin opens the control connection and authenticates, retrying only
// the rejections that a retry can actually fix.
func (t *NetSDKTransport) dialAndLogin() error {
	key := t.backoffKey()
	if v, ok := loginBackoff.Load(key); ok {
		if since := time.Since(v.(time.Time)); since < loginRetryBackoff {
			return fmt.Errorf("dahua: refusing to retry for another %s - these exact "+
				"credentials (addr|user|pass) were rejected %s ago. The camera locks this "+
				"session on bad logins (its LockLeftTime timer); every further attempt "+
				"re-extends that window. Correct the password/user to switch keys and retry immediately, or wait out the brake window",
				(loginRetryBackoff - since).Truncate(time.Second), since.Truncate(time.Second))
		}
		loginBackoff.Delete(key)
	}

	const attempts = 3
	for attempt := 1; ; attempt++ {
		var err error
		if t.ctrl, err = net.DialTimeout("tcp", t.addr, t.timeout); err != nil {
			return fmt.Errorf("dahua: dial %s: %w", t.addr, err)
		}

		if err = t.login(); err == nil {
			return nil
		}

		_ = t.ctrl.Close()
		t.ctrl = nil

		var refused *loginRefused
		if !errors.As(err, &refused) {
			return err
		}
		if refused.transient() && attempt < attempts {
			// Device is holding the previous session or busy; back off.
			time.Sleep(time.Duration(attempt) * 700 * time.Millisecond)
			continue
		}
		if !refused.transient() {
			loginBackoff.Store(key, time.Now())
		}
		return err
	}
}

func (t *NetSDKTransport) login() error {
	_ = t.ctrl.SetDeadline(time.Now().Add(t.timeout))

	if _, err := t.ctrl.Write(buildFrame(cmdLogin, nil, trailerChallenge[:])); err != nil {
		return err
	}

	hdr, body, err := readFrame(t.ctrl)
	if err != nil {
		return fmt.Errorf("dahua: no login challenge: %w", err)
	}
	if hdr[0] != cmdLoginResp {
		return fmt.Errorf("dahua: unexpected challenge reply 0x%02X", hdr[0])
	}

	kv := parseKV(body)
	realm, random := kv["Realm"], kv["Random"]
	if realm == "" || random == "" {
		return fmt.Errorf("dahua: malformed challenge %q", string(body))
	}

	payload := loginPayload(t.user, t.pass, realm, random)
	if _, err = t.ctrl.Write(buildFrame(cmdLogin, payload, trailerLogin[:])); err != nil {
		return err
	}

	if hdr, body, err = readFrame(t.ctrl); err != nil {
		return fmt.Errorf("dahua: no login reply: %w", err)
	}
	t.session = binary.LittleEndian.Uint32(hdr[16:20])
	if t.session == 0 {
		// hdr[8] carries the reason: 0x00 on accepted login, 0x01 on rejected
		// login (e.g. wrong password) — the same reason table the official client
		// decodes.
		return &loginRefused{code: hdr[8], reply: string(trimNUL(body))}
	}

	_ = t.ctrl.SetDeadline(time.Time{})
	return nil
}

// loginRefused is returned when the camera answers with session id 0, so Open
// can tell a transient rejection (busy / prior session held) from a permanent
// one (bad password, lockout) instead of retrying blindly.
type loginRefused struct {
	code  byte
	reply string
}

func (e *loginRefused) Error() string {
	return fmt.Sprintf("dahua: login refused: %s (code %d, reply %q)",
		loginErrorText(e.code), e.code, e.reply)
}

// transient reports whether retrying in a moment is likely to succeed:
//
//	4 - previous session not reaped yet
//	7 - device busy
//	8 - sub-connection limit reached
//
// All other codes fail identically on retry and only re-extend the session's
// LockLeftTime window, so they are not retried.
func (e *loginRefused) transient() bool {
	return e.code == 4 || e.code == 7 || e.code == 8
}

// loginErrorText maps hdr[8] of the 0xB0 login reply to a human reason.
// Codes 4/5/6 matter most: the device then keeps rejecting even the correct
// password until the lockout expires, which otherwise looks like a wrong password.
func loginErrorText(code byte) string {
	switch code {
	case 0:
		return "accepted but no session id returned"
	case 1:
		return "wrong password"
	case 2:
		return "user does not exist"
	case 3:
		return "timeout waiting for the login"
	case 4:
		return "account already logged in elsewhere"
	case 5:
		return "account locked"
	case 6:
		return "account in the blocklist (too many failed attempts - wait for the lockout to expire)"
	case 7:
		return "device is busy / resource limit"
	case 8:
		return "sub connection limit reached"
	case 9:
		return "no free channel"
	default:
		return "unknown reason"
	}
}

func (t *NetSDKTransport) addObject() (string, error) {
	if err := t.sendText(t.ctrl, []string{
		"TransactionID:6",
		"Method:AddObject",
		"ParameterName:Dahua.Device.Network.ControlConnection.Passive",
		"ConnectProtocol:0",
	}); err != nil {
		return "", err
	}

	kv, err := t.awaitText(t.ctrl, "AddObjectResponse")
	if err != nil {
		return "", fmt.Errorf("dahua: AddObject failed: %w", err)
	}
	if fc := kv["FaultCode"]; fc != "OK" && fc != "" {
		return "", fmt.Errorf("dahua: AddObject FaultCode=%s", fc)
	}

	cid := kv["ConnectionID"]
	if cid == "" {
		return "", fmt.Errorf("dahua: AddObject returned no ConnectionID")
	}
	return cid, nil
}

func (t *NetSDKTransport) ackSubChannel() error {
	if err := t.sendText(t.sub, []string{
		"TransactionID:0",
		"Method:GetParameterNames",
		"ParameterName:Dahua.Device.Network.ControlConnection.AckSubChannel",
		"SessionID:" + strconv.FormatUint(uint64(t.session), 10),
		"ConnectionID:" + t.connID,
		"Encrypt:0",
	}); err != nil {
		return err
	}

	kv, err := t.awaitText(t.sub, "AckSubChannel")
	if err != nil {
		return fmt.Errorf("dahua: sub channel not acknowledged: %w", err)
	}
	if kv["FaultCode"] != "OK" {
		return fmt.Errorf("dahua: AckSubChannel FaultCode=%s", kv["FaultCode"])
	}
	return nil
}

// setTalkState flips Dahua.Device.Network.Talk.General on or off. The stop
// request differs from the start one only by Depth/Frequency/State going to 0,
// exactly as observed in the dvrip_proxy capture of the vendor Talk.exe client.
func (t *NetSDKTransport) setTalkState(on bool, rate uint32) error {
	// Depth is hard-coded to "16" (bit depth) to match the demo's nAudioBit=16.
	// It is not derived from encodeFormat; a non-default encodeFormat value can
	// leave EncodeFormat and Depth inconsistent — only adjust if a device demands
	// it.
	depth, freq, state, tid := "16", strconv.FormatUint(uint64(rate), 10), "1", "7"
	if !on {
		depth, freq, state, tid = "0", "0", "0", "8"
	}

	// Read the control handle and connection id under the lock. keepAliveLoop
	// reads ctrl the same way (never reassigns it), and Open() publishes
	// connID under this same lock, so neither read races. t.channel /
	// t.encodeFormat are immutable after NewTransport, so reading them directly
	// is safe.
	t.mu.Lock()
	ctrl := t.ctrl
	connID := t.connID
	t.mu.Unlock()
	if ctrl == nil {
		return nil
	}

	return t.sendText(ctrl, []string{
		"TransactionID:" + tid,
		"Method:GetParameterNames",
		"ParameterName:Dahua.Device.Network.Talk.General",
		"Channel:" + strconv.Itoa(t.channel),
		"EncodeFormat:" + strconv.Itoa(t.encodeFormat),
		"Depth:" + depth,
		"Frequency:" + freq,
		"State:" + state,
		"ConnectionID:" + connID,
		"TalkMode:0",
	})
}

// ---------------------------------------------------------------------------
// background loops
// ---------------------------------------------------------------------------

func (t *NetSDKTransport) keepAliveLoop() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	// Capture the stop channel once. Open() may reassign t.stopKeep on a rare
	// same-instance reopen (the t.closed reset path), and reading the shared
	// field on every select iteration would race with that write. Watching a
	// local copy ties the loop to the channel it started with: the prior
	// Close() closes exactly this channel, so the loop still exits cleanly.
	stop := t.stopKeep

	frame := buildFrame(cmdKeepAlive, nil, nil)
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			t.mu.Lock()
			conn := t.ctrl
			t.mu.Unlock()
			if conn == nil {
				return
			}
			t.ctrlMu.Lock()
			_, err := conn.Write(frame)
			t.ctrlMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// receiveLoop drains the sub channel - mandatory even with no listener, or the
// camera stalls its send buffer and stops playing our audio.
// Downlink audio is consumed only when OnAudio is set (full duplex); otherwise
// each frame body is discarded via io.CopyN (no allocation), one frame ~40 ms.
func (t *NetSDKTransport) receiveLoop() {
	// Capture the stop channel once (same reasoning as keepAliveLoop): Open may
	// reassign t.stopKeep on a same-instance reopen, so watch a local copy.
	stop := t.stopKeep
	for {
		// Exit promptly on Close even if the device holds a half-open TCP
		// connection and goes silent (no RST, no data). Without this check the
		// loop would block in readHeader until the OS TCP timeout.
		select {
		case <-stop:
			return
		default:
		}

		// Bound the header read so a silent device cannot pin this goroutine
		// until the OS TCP timeout; the 2s deadline also re-runs the stop check
		// above regularly. A timeout here is expected on a quiet link, not an
		// error.
		_ = t.sub.SetReadDeadline(time.Now().Add(2 * time.Second))
		hdr, err := readHeader(t.sub)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return
		}
		_ = t.sub.SetReadDeadline(time.Time{})
		n := binary.LittleEndian.Uint32(hdr[4:8])
		if n > maxFrame {
			return
		}

		if hdr[0] == cmdTalkData && t.OnAudio != nil {
			body := make([]byte, n)
			if _, err := io.ReadFull(t.sub, body); err != nil {
				return
			}
			if payload, codecID := parseDHAVAudio(body); len(payload) > 0 {
				t.OnAudio(payload, codecID)
			}
			continue
		}

		if n > 0 {
			if _, err := io.CopyN(io.Discard, t.sub, int64(n)); err != nil {
				return
			}
		}
	}
}

// readHeader reads just the 32 byte frame header off a connection. receiveLoop
// uses it when the body is not needed.
func readHeader(conn net.Conn) ([]byte, error) {
	if conn == nil {
		return nil, io.ErrClosedPipe
	}
	hdr := make([]byte, talkHeaderSize)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return nil, err
	}
	return hdr, nil
}

// ---------------------------------------------------------------------------
// framing helpers
// ---------------------------------------------------------------------------

// buildFrame assembles a 32 byte header plus body. Only the login command sets
// the 05 00 60 magic in header[1:4]; others leave it zero (verified in capture).
func buildFrame(cmd byte, body, trailer []byte) []byte {
	buf := make([]byte, talkHeaderSize+len(body))
	buf[0] = cmd
	if cmd == cmdLogin {
		buf[1], buf[2], buf[3] = 0x05, 0x00, 0x60
	}
	binary.LittleEndian.PutUint32(buf[4:], uint32(len(body)))
	if len(trailer) == 8 {
		copy(buf[24:32], trailer)
	}
	copy(buf[talkHeaderSize:], body)
	return buf
}

// g711ToPCM16 expands a G.711 payload to 16-bit LE PCM, so a WebRTC browser
// (which can only offer Opus/G.711) is audible through the PCM16 speaker.
func g711ToPCM16(payload []byte, from string) []byte {
	out := make([]byte, len(payload)*2)
	if from == core.CodecPCMU {
		for i, b := range payload {
			binary.LittleEndian.PutUint16(out[i*2:], uint16(pcm.PCMUtoPCM(b)))
		}
		return out
	}
	for i, b := range payload {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(pcm.PCMAtoPCM(b)))
	}
	return out
}

// buildTalkAudio wraps a payload in the 0x1D talk frame. The four little
// endian fields in the header describe the raw stream, the DHAV sub-header
// describes this particular packet.
func buildTalkAudio(payload []byte, codecID, rateIndex byte, rate uint32) []byte {
	buf := make([]byte, talkHeaderSize+8+len(payload))
	buf[0] = cmdTalkData
	binary.LittleEndian.PutUint32(buf[4:], uint32(8+len(payload)))
	buf[8] = 0x02 // audio
	// buf[9:13] is the wire "bits per sample" field. The camera appears to
	// ignore it for talk audio: the format is governed by the DHAV sub-header
	// codec id (sub[4]), not this outer field. The value is pinned to 16 by a
	// PCM16 capture; do NOT retune it per codec without re-confirming against a
	// real capture, or the A-law passthrough path could regress.
	binary.LittleEndian.PutUint32(buf[9:], 16)
	binary.LittleEndian.PutUint32(buf[13:], 1)
	binary.LittleEndian.PutUint32(buf[17:], rate)

	sub := buf[talkHeaderSize:]
	sub[0], sub[1], sub[2], sub[3] = 0x00, 0x00, 0x01, 0xF0
	sub[4] = codecID
	sub[5] = rateIndex
	binary.LittleEndian.PutUint16(sub[6:], uint16(len(payload)))
	copy(sub[8:], payload)

	return buf
}

func readFrame(conn net.Conn) ([]byte, []byte, error) {
	if conn == nil {
		return nil, nil, io.ErrClosedPipe
	}

	hdr := make([]byte, talkHeaderSize)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return nil, nil, err
	}

	n := binary.LittleEndian.Uint32(hdr[4:8])
	if n > maxFrame {
		return nil, nil, fmt.Errorf("dahua: bogus frame length %d", n)
	}
	if n == 0 {
		return hdr, nil, nil
	}

	body := make([]byte, n)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, nil, err
	}
	return hdr, body, nil
}

func (t *NetSDKTransport) sendText(conn net.Conn, lines []string) error {
	if conn == nil {
		return io.ErrClosedPipe
	}
	body := []byte(strings.Join(lines, "\r\n") + "\r\n\r\n")
	frame := buildFrame(cmdText, body, nil)
	if t.Debug != nil {
		t.Debug("text ->", body)
	}
	// Writes to the sub channel compete with WriteAudio (which holds t.mu);
	// writes to the control channel compete with keepAliveLoop (which holds
	// t.ctrlMu). Lock the one that matches this connection so the two byte
	// streams never interleave.
	if conn == t.sub {
		t.mu.Lock()
		_, err := conn.Write(frame)
		t.mu.Unlock()
		return err
	}
	t.ctrlMu.Lock()
	_, err := conn.Write(frame)
	t.ctrlMu.Unlock()
	return err
}

// awaitText reads until a 0xF4 frame whose body mentions want shows up.
func (t *NetSDKTransport) awaitText(conn net.Conn, want string) (map[string]string, error) {
	_ = conn.SetReadDeadline(time.Now().Add(t.timeout))
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	for i := 0; i < 32; i++ {
		hdr, body, err := readFrame(conn)
		if err != nil {
			return nil, err
		}
		if hdr[0] != cmdText || !strings.Contains(string(body), want) {
			continue
		}
		if t.Debug != nil {
			t.Debug("text <-", body)
		}
		return parseKV(body), nil
	}
	return nil, fmt.Errorf("dahua: no %s reply", want)
}

func parseKV(body []byte) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimRight(string(body), "\x00\r\n"), "\r\n") {
		if i := strings.IndexByte(line, ':'); i > 0 {
			out[strings.TrimSpace(line[:i])] = strings.TrimSpace(line[i+1:])
		}
	}
	return out
}

// parseDHAVAudio decodes a downstream 0x1D body.
//
// The camera does not echo the short 8 byte sub-header we send upstream. It
// answers with a complete DHAV container (368 bytes for 40 ms of G.711A):
//
//	0   'DHAV'
//	4   0xF0                 frame type audio
//	5   subtype  6 channel  7 subchannel
//	8   uint32 LE            frame sequence, +1 per frame
//	12  uint32 LE            total length, tail included
//	16  uint32 LE            date (packed Y/M/D/H/M/S, see FFmpeg get_timeinfo)
//	20  uint16 LE            timestamp (+40 per frame, see FFmpeg get_pts)
//	22  uint8                extension length, 0x10 observed
//	24  extension            0x83 <flag> <codec id> <rate index> ...
//	24+extLen                raw audio payload
//	len-8                    'dhav' + uint32 LE total length
func parseDHAVAudio(body []byte) ([]byte, byte) {
	if len(body) < 32 || string(body[0:4]) != "DHAV" || body[4] != dhavTypeAudio {
		// Fallback for firmware that echoes the short upstream talk header
		// instead of a full DHAV container. The short header is exactly
		// 00 00 01 F0 at [0:4], codec id at [4], payload at [8:]. We only
		// accept that known shape, so a stray non-DHAV frame can never be
		// silently mis-parsed as audio (which would feed garbage to the decoder).
		if len(body) > 8 &&
			body[0] == 0x00 && body[1] == 0x00 && body[2] == 0x01 && body[3] == 0xF0 {
			return body[8:], body[4]
		}
		return nil, 0
	}

	start := 24 + int(body[22])
	end := len(body) - 8
	if end < 0 || string(body[end:end+4]) != "dhav" {
		// No trailing 'dhav'+length marker: the audio boundary is unknown, so
		// refuse the frame rather than feed its 8-byte trailer region to the
		// decoder as audio (which would inject garbage before the next frame).
		return nil, 0
	}
	if start >= end {
		return nil, 0
	}

	codecID := byte(0x0E) // G.711A, what the E4702 microphone sends
	// FFmpeg dhav.c parse_ext: the audio extension type sits at body[24]; the
	// codec id is at body[26] for type 0x83 and at body[27] for type 0x8c.
	switch {
	case int(body[22]) >= 4 && body[24] == 0x83:
		codecID = body[26]
	case int(body[22]) >= 4 && body[24] == 0x8c:
		codecID = body[27]
	}
	return body[start:end], codecID
}

// ---------------------------------------------------------------------------
// login hashes
// ---------------------------------------------------------------------------

// loginPayload builds the 71 byte body of the second 0xA0 frame:
//
//	<user>&&<gen2 32hex><gen1 32hex>
//
// gen2 is the modern two-stage MD5; gen1 the legacy 8-byte scrambled hash run
// through one more MD5 with the random. Verified against mcw0/DahuaConsole
// dahua_logon_modes.py (logon='dvrip') - the only open implementation; a bare
// gen2 hash is rejected silently, looking exactly like a dead port.
func loginPayload(user, pass, realm, random string) []byte {
	gen2 := upperMD5(user + ":" + realm + ":" + pass)
	gen2 = upperMD5(user + ":" + random + ":" + gen2)
	gen1 := upperMD5(user + ":" + random + ":" + gen1Hash(pass))
	return []byte(user + "&&" + gen2 + gen1)
}

// gen1Hash is the Dahua legacy "OldDigest": raw MD5 of the password, each
// adjacent byte pair summed mod 62 and mapped to 0-9/A-Z/a-z. From
// mcw0/DahuaConsole _compressor, tracing back to haicen/DahuaHashCreator.
func gen1Hash(pass string) string {
	sum := md5.Sum([]byte(pass))
	out := make([]byte, 8)
	for i := 0; i < 8; i++ {
		v := (int(sum[i*2]) + int(sum[i*2+1])) % 62
		switch {
		case v < 10:
			v += 48 // '0'-'9'
		case v < 36:
			v += 55 // 'A'-'Z'
		default:
			v += 61 // 'a'-'z'
		}
		out[i] = byte(v)
	}
	return string(out)
}

func upperMD5(s string) string {
	sum := md5.Sum([]byte(s))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// trimNUL drops the NUL and CRLF padding Dahua appends to the text frames of
// the talk channel.
func trimNUL(b []byte) []byte {
	return bytes.TrimRight(b, "\x00\r\n")
}

// ---------------------------------------------------------------------------
// codec / credential helpers
// ---------------------------------------------------------------------------

// payloadType maps a codec name to its static RTP payload type. NetSDK does not
// actually use RTP, but the value is handy when describing the codec to the
// go2rtc negotiator.
func payloadType(name string) uint8 {
	switch name {
	case core.CodecPCMA:
		return 8
	case core.CodecPCMU:
		return 0
	default:
		return core.PayloadTypeRAW
	}
}

// passFingerprint is the same one way 8 character compression the login hash
// uses, so a password can be printed and compared without ever revealing it.
func passFingerprint(pass string) string {
	return gen1Hash(pass)
}
