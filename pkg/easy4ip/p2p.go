package easy4ip

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Easy4IP P2P handshake of Dahua/KBVision devices (DMSS, Imou...), ported from dh-p2p (MIT).
// Rust used panics for every failure; here each one is an error the supervisor retries.

// mainServer is a variable only so tests can point it at a fake.
var mainServer = "www.easy4ipcloud.com:8800"

const (
	// username/key of the public Dahua app registration, same as in upstream dh-p2p
	wsseUser = "cba1b29e32cb17aa46b8ff9e73c7f40b"
	wsseKey  = "996103384cdf19179e19243e959bbf8b"
)

// HandshakeTimeout bounds a whole handshake. Do NOT shorten it: the handshake opens a relay leg that
// the device keeps ~30 s and cannot be told to close. A new attempt inside that window hangs at
// /relay-channel, and every aborted attempt leaves one more leg, so quick retries keep the device
// busy forever (measured: 10 s timeouts never connected).
const HandshakeTimeout = 30 * time.Second

// Logf receives protocol traces; nil is silent. The traces carry the serial and auth data, so a
// caller should only enable it for debugging.
var Logf func(format string, args ...any)

// Warnf receives failures worth showing to a user (handshake errors, a silent device). They carry no
// serial or auth data.
var Warnf func(format string, args ...any)

func warnf(format string, args ...any) {
	if f := Warnf; f != nil {
		f(format, args...)
	}
}

func logf(format string, args ...any) {
	if f := Logf; f != nil {
		f(format, args...)
	}
}

// clockOffset is the server clock minus ours in seconds, learnt from the Date header of every reply.
// The WSSE Created stamp must be close to server time or every request gets 401 <Error>TimeOut</Error>,
// and TV boxes often run with a wrong clock. The first request after a skewed start fails, the retry
// is signed with the corrected time.
var clockOffset atomic.Int64

func serverNow() time.Time {
	return time.Now().Add(time.Duration(clockOffset.Load()) * time.Second).UTC()
}

// Cred are the device login for signed channel requests, the way DMSS sends them. With a stable
// ClientID the device takes a reconnect as the same client and drops our stale session at once;
// unsigned, it keeps the previous relay leg ~30 s and a new /relay-channel hangs until then.
type Cred struct {
	User     string
	Password string
	ClientID string // DMSS sends "<install UUID>:37777"
}

// devAuth builds <CreateDate/><DevAuth/><Nonce/><UserName/> for a channel request.
// DevAuth = base64(HMAC-SHA256(key, nonce+created+payload)), key = HEX(MD5("user:Login to salt:password")).
// ponytail: empty salt only (what /info/device gives for our XVR); firmwares with a per-device randsalt
// need /info/device decrypted first (see blackjid/easy4ip dh/crypto.go).
func devAuth(c *Cred, payload string) string {
	var n [4]byte
	_, _ = rand.Read(n[:])
	nonce := int32(binary.BigEndian.Uint32(n[:]))
	created := serverNow().Unix()
	return fmt.Sprintf("<CreateDate>%d</CreateDate><DevAuth>%s</DevAuth><Nonce>%d</Nonce><UserName>%s</UserName>",
		created, devAuthDigest(c.User, c.Password, nonce, created, payload), nonce, c.User)
}

func devAuthDigest(user, password string, nonce int32, created int64, payload string) string {
	sum := md5.Sum([]byte(user + ":Login to :" + password))
	key := strings.ToUpper(hex.EncodeToString(sum[:]))
	mac := hmac.New(sha256.New, []byte(key))
	fmt.Fprintf(mac, "%d%d%s", nonce, created, payload)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// peerConn is a UDP socket that behaves like the Rust "connected" one: it keeps its local port while
// the peer changes, and drops datagrams from anyone else.
type peerConn struct {
	*net.UDPConn
	peer *net.UDPAddr
}

func listen() (*peerConn, error) {
	c, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, err
	}
	return &peerConn{UDPConn: c}, nil
}

func (c *peerConn) connect(addr string) error {
	a, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return err
	}
	c.peer = a
	return nil
}

func (c *peerConn) Write(b []byte) (int, error) { return c.UDPConn.WriteToUDP(b, c.peer) }

func (c *peerConn) Read(b []byte) (int, error) {
	for {
		n, from, err := c.UDPConn.ReadFromUDP(b)
		if err != nil {
			return 0, err
		}
		if from.Port == c.peer.Port && from.IP.Equal(c.peer.IP) {
			return n, nil
		}
	}
}

// readPacket returns the next well-formed PTCP packet (copied out of the read buffer).
func (c *peerConn) readPacket() (*packet, error) {
	buf := make([]byte, 65536)
	for {
		n, err := c.Read(buf)
		if err != nil {
			return nil, err
		}
		if p, ok := parsePacket(append([]byte(nil), buf[:n]...)); ok {
			return p, nil
		}
	}
}

func (c *peerConn) writePacket(p *packet) error {
	_, err := c.Write(p.marshal())
	return err
}

// response is a parsed Easy4IP reply: "HTTP/1.1 200 OK" headers, then a flat XML body.
type response struct {
	code    int
	status  string
	headers map[string]string
	body    map[string]string // element path -> text, e.g. "body/US"
}

// ResponseError is a reply with a code >= 300 from the Easy4IP servers or the device.
type ResponseError struct {
	Code   int
	Status string
}

func (e *ResponseError) Error() string { return fmt.Sprintf("easy4ip: %d %s", e.Code, e.Status) }

func (r *response) get(path string) (string, error) {
	v, ok := r.body[path]
	if !ok {
		return "", fmt.Errorf("easy4ip: reply has no %s", path)
	}
	return v, nil
}

func parseXML(s string) (map[string]string, error) {
	dec := xml.NewDecoder(strings.NewReader(s))
	var stack []string
	tree := map[string]string{}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return tree, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if v := string(t); strings.TrimSpace(v) != "" {
				tree[strings.Join(stack, "/")] = v
			}
		}
	}
}

func parseResponse(s string) (*response, error) {
	head, body, _ := strings.Cut(s, "\r\n\r\n")
	lines := strings.Split(head, "\r\n")
	parts := strings.SplitN(lines[0], " ", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("easy4ip: bad status line %q", lines[0])
	}
	code, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("easy4ip: bad status line %q", lines[0])
	}
	r := &response{code: code, headers: map[string]string{}}
	if len(parts) == 3 {
		r.status = parts[2]
	}
	for _, line := range lines[1:] {
		if k, v, ok := strings.Cut(line, ": "); ok {
			r.headers[k] = v
		}
	}
	if strings.TrimSpace(body) != "" {
		if r.body, err = parseXML(body); err != nil {
			return nil, err
		}
	}
	return r, nil
}

type dhConn struct {
	*peerConn
	cseq *int
}

// request sends a WSSE-signed DHGET (no body) or DHPOST.
func (c dhConn) request(path, body string) error {
	method := "DHGET"
	if body != "" {
		method = "DHPOST"
	}

	var n [4]byte
	_, _ = rand.Read(n[:])
	nonce := binary.BigEndian.Uint32(n[:])
	created := serverNow().Format("2006-01-02T15:04:05Z")

	sum := sha1.Sum([]byte(fmt.Sprintf("%d%sDHP2P:%s:%s", nonce, created, wsseUser, wsseKey)))
	digest := base64.StdEncoding.EncodeToString(sum[:])

	*c.cseq++
	req := fmt.Sprintf("%s %s HTTP/1.1\r\nCSeq: %d\r\nAuthorization: WSSE profile=\"UsernameToken\"\r\n"+
		"X-WSSE: UsernameToken Username=\"%s\", PasswordDigest=\"%s\", Nonce=\"%d\", Created=\"%s\"\r\n\r\n%s",
		method, path, *c.cseq, wsseUser, digest, nonce, created, body)

	logf("easy4ip >>> %s %s", c.peer, path)
	_, err := c.Write([]byte(req))
	return err
}

// readRaw reads one reply whatever its code.
func (c dhConn) readRaw() (*response, error) {
	buf := make([]byte, 4096)
	n, err := c.Read(buf)
	if err != nil {
		return nil, err
	}
	r, err := parseResponse(string(buf[:n]))
	if err != nil {
		return nil, err
	}
	logf("easy4ip <<< %s %d %s", c.peer, r.code, r.status)

	if d, err := time.Parse(time.RFC3339, r.headers["Date"]); err == nil {
		clockOffset.Store(d.Unix() - time.Now().Unix())
	}
	return r, nil
}

// read reads one reply and fails on a code >= 300.
func (c dhConn) read() (*response, error) {
	r, err := c.readRaw()
	if err != nil {
		return nil, err
	}
	if r.code >= 300 {
		return nil, &ResponseError{r.code, r.status}
	}
	return r, nil
}

// ipBytes: "ip:port" as port (2 bytes BE) + ip, every byte inverted.
func ipBytes(addr string) ([]byte, error) {
	a, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, err
	}
	ip := a.IP.To4()
	if ip == nil {
		return nil, fmt.Errorf("easy4ip: %s is not IPv4", addr)
	}
	b := []byte{byte(a.Port >> 8), byte(a.Port), ip[0], ip[1], ip[2], ip[3]}
	for i := range b {
		b[i] = ^b[i]
	}
	return b, nil
}

// openSession opens a PTCP session to the device. With relay the session runs through the Dahua relay
// (experimental, slow); otherwise the relay only supplies a "sign" and the traffic goes directly.
// The returned socket is connected to the device (or relay agent) and has no deadlines.
func openSession(serial string, relay bool, cred *Cred) (*peerConn, *ptcp, error) {
	deadline := time.Now().Add(HandshakeTimeout)

	s1, err := listen()
	if err != nil {
		return nil, nil, err
	}
	s2, err := listen()
	if err != nil {
		_ = s1.Close()
		return nil, nil, err
	}
	// whatever happens, s2 (the relay leg) is not needed afterwards unless relay mode keeps it
	keep := s2
	if !relay {
		keep = s1
	}
	ok := false
	defer func() {
		if ok {
			_ = s1.SetDeadline(time.Time{})
			_ = s2.SetDeadline(time.Time{})
		}
		if !ok || keep != s1 {
			_ = s1.Close()
		}
		if !ok || keep != s2 {
			_ = s2.Close()
		}
	}()
	_ = s1.SetDeadline(deadline)
	_ = s2.SetDeadline(deadline)

	sess, err := handshake(s1, s2, serial, relay, cred, deadline)
	if err != nil {
		return nil, nil, err
	}
	ok = true
	return keep, sess, nil
}

func handshake(s1, s2 *peerConn, serial string, relay bool, cred *Cred, deadline time.Time) (*ptcp, error) {
	cseq := 0
	d1, d2 := dhConn{s1, &cseq}, dhConn{s2, &cseq}

	// signed fields go first, ClientId last, as in DMSS's requests
	signed := func(payload string) (string, string) {
		if cred == nil {
			return "", ""
		}
		return devAuth(cred, payload), "<ClientId>" + cred.ClientID + "</ClientId>"
	}

	// ask does one request/response on a socket
	ask := func(c dhConn, path, body string) (*response, error) {
		if err := c.request(path, body); err != nil {
			return nil, err
		}
		return c.read()
	}

	if err := s1.connect(mainServer); err != nil {
		return nil, err
	}
	if _, err := ask(d1, "/probe/p2psrv", ""); err != nil {
		return nil, err
	}
	res, err := ask(d1, "/online/p2psrv/"+serial, "")
	if err != nil {
		return nil, err
	}
	p2psrv, err := res.get("body/US")
	if err != nil {
		return nil, err
	}
	if res, err = ask(d1, "/online/relay", ""); err != nil {
		return nil, err
	}
	relayAddr, err := res.get("body/Address")
	if err != nil {
		return nil, err
	}

	if err = s2.connect(p2psrv); err != nil {
		return nil, err
	}
	if _, err = ask(d2, "/probe/device/"+serial, ""); err != nil {
		return nil, err
	}

	var cidRaw [8]byte
	_, _ = rand.Read(cidRaw[:])
	var cidText []string
	for _, b := range cidRaw {
		cidText = append(cidText, fmt.Sprintf("%x", b)) // no zero padding, as the device expects
	}
	localAddr := fmt.Sprintf("127.0.0.1:%d", s1.LocalAddr().(*net.UDPAddr).Port)
	auth, clientTag := signed(localAddr)

	// the answer to this one comes later, once the device has been told to connect
	if err = d1.request("/device/"+serial+"/p2p-channel", fmt.Sprintf(
		"<body>%s<Identify>%s</Identify><IpEncrpt>true</IpEncrpt><LocalAddr>%s</LocalAddr><version>5.0.0</version>%s</body>",
		auth, strings.Join(cidText, " "), localAddr, clientTag)); err != nil {
		return nil, err
	}

	if err = s2.connect(relayAddr); err != nil {
		return nil, err
	}
	if res, err = ask(d2, "/relay/agent", ""); err != nil {
		return nil, err
	}
	token, err := res.get("body/Token")
	if err != nil {
		return nil, err
	}
	agent, err := res.get("body/Agent")
	if err != nil {
		return nil, err
	}

	if err = s2.connect(agent); err != nil {
		return nil, err
	}
	if _, err = ask(d2, "/relay/start/"+token, "<body><Client>:0</Client></body>"); err != nil {
		return nil, err
	}

	// the p2p-channel answer, maybe preceded by a 100
	if res, err = d1.readRaw(); err != nil {
		return nil, err
	}
	if res.code == 100 {
		if res, err = d1.readRaw(); err != nil {
			return nil, err
		}
	}
	if res.code >= 400 {
		return nil, &ResponseError{res.code, res.status} // 403: the device wants credentials we did not send
	}
	deviceLocal, err := res.get("body/LocalAddr")
	if err != nil {
		return nil, err
	}
	device, err := res.get("body/PubAddr")
	if err != nil {
		return nil, err
	}

	// not necessary in relay mode, but UDP is connectionless
	if err = s1.connect(device); err != nil {
		return nil, err
	}

	if err = s2.connect(mainServer); err != nil {
		return nil, err
	}
	auth, clientTag = signed("")
	if err = d2.request("/device/"+serial+"/relay-channel",
		"<body>"+auth+clientTag+"<agentAddr>"+agent+"</agentAddr></body>"); err != nil {
		return nil, err
	}
	if err = s2.connect(agent); err != nil {
		return nil, err
	}
	if _, err = d2.read(); err != nil { // hangs here while the device still holds a previous relay leg
		return nil, err
	}

	sess := newPTCP()
	if err = s2.writePacket(sess.send(&body{kind: kindSync})); err != nil {
		return nil, err
	}
	if _, err = recvSync(s2, sess); err != nil {
		return nil, err
	}
	if relay {
		return sess, nil
	}

	// the sign that authorises the direct session
	if err = s2.writePacket(sess.send(&body{kind: kindCommand, cmd: cmd(0x17, nil)})); err != nil {
		return nil, err
	}
	signPkt, err := recvSyncSkipEmpty(s2, sess)
	if err != nil {
		return nil, err
	}
	if signPkt.body.kind != kindCommand || len(signPkt.body.cmd) < 12 {
		return nil, errors.New("easy4ip: bad sign reply")
	}
	sign := signPkt.body.cmd[12:]

	// NAT traversal: two raw datagrams to the device's public and local address
	var cookie [4]byte
	var transID [12]byte
	_, _ = rand.Read(cookie[:])
	_, _ = rand.Read(transID[:])
	cid := make([]byte, 8)
	for i, b := range cidRaw {
		cid[i] = ^b
	}
	devIP, err := ipBytes(device)
	if err != nil {
		return nil, err
	}
	devLocalIP, err := ipBytes(deviceLocal)
	if err != nil {
		return nil, err
	}

	punch := bytes.Join([][]byte{{0xff, 0xfe, 0xff, 0xe7}, cookie[:], transID[:], {0x7f, 0xd5, 0xff, 0xf7}, cid, {0xff, 0xfb, 0xff, 0xf7, 0xff, 0xfe}, devIP}, nil)
	if _, err = s1.Write(punch); err != nil {
		return nil, err
	}

	buf := make([]byte, 4096)
	_ = s1.SetReadDeadline(minTime(deadline, time.Now().Add(5*time.Second)))
	n, err := s1.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("easy4ip: no answer to the direct connect (a device behind a strict NAT may need relay mode): %w", err)
	}
	_ = s1.SetReadDeadline(deadline)
	if n < 20 {
		return nil, errors.New("easy4ip: short direct connect answer")
	}
	rTransID := append([]byte(nil), buf[8:20]...)

	punch = bytes.Join([][]byte{{0xfe, 0xfe, 0xff, 0xe7}, cookie[:], rTransID, {0x7f, 0xd6, 0xff, 0xf7}, cid, {0xff, 0xfb, 0xff, 0xf7, 0xff, 0xfe}, devLocalIP}, nil)
	if _, err = s1.Write(punch); err != nil {
		return nil, err
	}
	for i := 0; i < 5; i++ { // the device sends five more datagrams we have no use for
		if _, err = s1.Read(buf); err != nil {
			return nil, err
		}
	}

	sess = newPTCP()
	if err = s1.writePacket(sess.send(&body{kind: kindSync})); err != nil {
		return nil, err
	}
	p, err := recvSync(s1, sess)
	if err != nil {
		return nil, err
	}
	if p.body.kind != kindSync {
		return nil, errors.New("easy4ip: device did not answer Sync")
	}

	if err = s1.writePacket(sess.send(&body{kind: kindCommand, cmd: cmd(0x19, sign)})); err != nil {
		return nil, err
	}
	if p, err = recvSyncSkipEmpty(s1, sess); err != nil {
		return nil, err
	}
	if p.body.kind != kindCommand || len(p.body.cmd) == 0 || p.body.cmd[0] != 0x1A {
		return nil, errors.New("easy4ip: device refused the sign")
	}

	if err = s1.writePacket(sess.send(&body{kind: kindCommand, cmd: cmd(0x1b, nil)})); err != nil {
		return nil, err
	}
	if p, err = recvSync(s1, sess); err != nil {
		return nil, err
	}
	if p.body.kind != kindEmpty {
		return nil, errors.New("easy4ip: unexpected answer to the final command")
	}

	// the traffic of this session is s1; the caller keeps whichever socket carries it
	return sess, nil
}

// cmd is a 12-byte command header (type byte, rest zero) followed by data.
func cmd(typ byte, data []byte) []byte {
	return append(append([]byte{typ}, make([]byte, 11)...), data...)
}

func recvSync(c *peerConn, s *ptcp) (*packet, error) {
	p, err := c.readPacket()
	if err != nil {
		return nil, err
	}
	return s.recvSync(p), nil
}

// recvSyncSkipEmpty skips the bare acks that can arrive before the answer.
func recvSyncSkipEmpty(c *peerConn, s *ptcp) (*packet, error) {
	for {
		p, err := recvSync(c, s)
		if err != nil || p.body.kind != kindEmpty {
			return p, err
		}
	}
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
