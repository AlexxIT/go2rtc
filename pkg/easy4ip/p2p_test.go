package easy4ip

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDevAuthDigest(t *testing.T) {
	// computed independently of this code (Python), and equal to what a DMSS capture implies
	require.Equal(t, "WZQ+vty35rSihfijfW1ibWaxfm6UEtpEr6YHPzR1RBM=",
		devAuthDigest("admin", "secret", -123456789, 1791815007, "127.0.0.1:5000"))
}

func TestIPBytes(t *testing.T) {
	b, err := ipBytes("192.168.1.2:37777") // 37777 = 0x9291
	require.NoError(t, err)
	require.Equal(t, []byte{^byte(0x93), ^byte(0x91), ^byte(192), ^byte(168), ^byte(1), ^byte(2)}, b)

	_, err = ipBytes("[::1]:80")
	require.Error(t, err)
}

func TestParseResponse(t *testing.T) {
	r, err := parseResponse("HTTP/1.1 200 OK\r\nCSeq: 3\r\nDate: 2026-10-13T10:00:00Z\r\n\r\n" +
		"<?xml version=\"1.0\"?>\n<body>\n  <US>1.2.3.4:8800</US>\n  <Relay><Address>5.6.7.8:9</Address></Relay>\n</body>")
	require.NoError(t, err)
	require.Equal(t, 200, r.code)
	require.Equal(t, "OK", r.status)
	require.Equal(t, "2026-10-13T10:00:00Z", r.headers["Date"])
	require.Equal(t, "1.2.3.4:8800", r.body["body/US"])
	require.Equal(t, "5.6.7.8:9", r.body["body/Relay/Address"])
	require.NotContains(t, r.body, "body", "whitespace between elements is not a value")

	r, err = parseResponse("HTTP/1.1 401 Unauthorized\r\n\r\n<body><Error>TimeOut</Error></body>")
	require.NoError(t, err)
	require.Equal(t, "TimeOut", r.body["body/Error"])

	r, err = parseResponse("HTTP/1.1 100 Trying\r\n\r\n")
	require.NoError(t, err)
	require.Nil(t, r.body)

	for _, bad := range []string{"", "garbage", "HTTP/1.1 abc OK\r\n\r\n", "HTTP/1.1 200 OK\r\n\r\n<body><x></body>"} {
		_, err = parseResponse(bad)
		require.Error(t, err, bad)
	}
}

// fakeNetwork stands in for the Easy4IP servers, the relay and the device, to run the whole direct
// handshake offline. One socket plays every server (it answers by request path), one the device.
type fakeNetwork struct {
	server, device *net.UDPConn
	signSeen       chan string
	chanReqs       chan string
}

func newFakeNetwork(t *testing.T) *fakeNetwork {
	listen := func() *net.UDPConn {
		c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		require.NoError(t, err)
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	f := &fakeNetwork{server: listen(), device: listen(), signSeen: make(chan string, 1), chanReqs: make(chan string, 8)}
	go f.serveServer()
	go f.serveDevice()
	return f
}

func xmlReply(code int, status, body string) []byte {
	return []byte(fmt.Sprintf("HTTP/1.1 %d %s\r\nDate: 2030-01-01T00:00:00Z\r\n\r\n%s", code, status, body))
}

func (f *fakeNetwork) serveServer() {
	srv := f.server.LocalAddr().String()
	dev := f.device.LocalAddr().String()
	buf := make([]byte, 65536)
	for {
		n, from, err := f.server.ReadFromUDP(buf)
		if err != nil {
			return
		}
		req := string(buf[:n])
		reply := func(b []byte) { _, _ = f.server.WriteToUDP(b, from) }

		switch {
		case strings.HasPrefix(req, "PTCP"): // the relay leg
			p, ok := parsePacket(buf[:n])
			if !ok {
				continue
			}
			switch {
			case p.body.kind == kindSync:
				reply((&packet{body: &body{kind: kindSync}}).marshal())
			case p.body.kind == kindCommand && p.body.cmd[0] == 0x17:
				reply((&packet{body: &body{kind: kindCommand, cmd: cmd(0x18, []byte("SIGN"))}}).marshal())
			}
		case strings.Contains(req, "/probe/"):
			reply(xmlReply(200, "OK", ""))
		case strings.Contains(req, "/online/p2psrv/"):
			reply(xmlReply(200, "OK", "<body><US>"+srv+"</US></body>"))
		case strings.Contains(req, "/online/relay"):
			reply(xmlReply(200, "OK", "<body><Address>"+srv+"</Address></body>"))
		case strings.Contains(req, "/relay/agent"):
			reply(xmlReply(200, "OK", "<body><Token>TOK</Token><Agent>"+srv+"</Agent></body>"))
		case strings.Contains(req, "/relay/start/TOK"):
			reply(xmlReply(200, "OK", ""))
		case strings.Contains(req, "/p2p-channel"):
			f.chanReqs <- req
			reply(xmlReply(100, "Trying", ""))
			reply(xmlReply(200, "OK", "<body><LocalAddr>"+dev+"</LocalAddr><PubAddr>"+dev+"</PubAddr></body>"))
		case strings.Contains(req, "/relay-channel"):
			f.chanReqs <- req
			reply(xmlReply(200, "OK", ""))
		}
	}
}

func (f *fakeNetwork) serveDevice() {
	buf := make([]byte, 65536)
	for {
		n, from, err := f.device.ReadFromUDP(buf)
		if err != nil {
			return
		}
		reply := func(b []byte) { _, _ = f.device.WriteToUDP(b, from) }
		d := buf[:n]

		switch {
		case n >= 4 && string(d[:4]) == "\xff\xfe\xff\xe7": // first punch: answer with a transaction id
			b := make([]byte, 32)
			copy(b[8:20], "TRANSACTION!")
			reply(b)
		case n >= 4 && string(d[:4]) == "\xfe\xfe\xff\xe7": // second punch: five datagrams follow
			for i := 0; i < 5; i++ {
				reply([]byte("filler"))
			}
		case n >= 4 && string(d[:4]) == "PTCP":
			p, ok := parsePacket(d)
			if !ok {
				continue
			}
			switch {
			case p.body.kind == kindSync:
				reply((&packet{body: &body{kind: kindSync}}).marshal())
			case p.body.kind == kindCommand && p.body.cmd[0] == 0x19:
				f.signSeen <- string(p.body.cmd[12:])
				reply((&packet{body: &body{kind: kindCommand, cmd: cmd(0x1A, nil)}}).marshal())
			case p.body.kind == kindCommand && p.body.cmd[0] == 0x1b:
				reply((&packet{body: &body{kind: kindEmpty}}).marshal())
			}
		}
	}
}

func TestHandshakeDirect(t *testing.T) {
	f := newFakeNetwork(t)
	mainServer = f.server.LocalAddr().String()
	t.Cleanup(func() { mainServer = "www.easy4ipcloud.com:8800"; clockOffset.Store(0) })

	conn, sess, err := openSession("SERIAL123", false, &Cred{User: "admin", Password: "pw", ClientID: "uuid:37777"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	require.Equal(t, f.device.LocalAddr().String(), conn.peer.String(), "traffic goes to the device, not the relay")
	require.Equal(t, "SIGN", <-f.signSeen, "the sign from the relay leg authorises the direct session")
	require.Positive(t, sess.sent, "the session has sent its handshake commands")

	// both channel requests are signed and tagged
	for i := 0; i < 2; i++ {
		req := <-f.chanReqs
		require.Contains(t, req, "<DevAuth>", req)
		require.Contains(t, req, "<UserName>admin</UserName>")
		require.Contains(t, req, "<ClientId>uuid:37777</ClientId>")
	}

	// the Date header of the replies moved our notion of server time (2030 vs now)
	require.Greater(t, clockOffset.Load(), int64(3*365*24*3600))
	require.Greater(t, serverNow().Year(), 2029)
}

func TestHandshakeRelay(t *testing.T) {
	f := newFakeNetwork(t)
	mainServer = f.server.LocalAddr().String()
	t.Cleanup(func() { mainServer = "www.easy4ipcloud.com:8800"; clockOffset.Store(0) })

	conn, _, err := openSession("SERIAL123", true, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.Equal(t, f.server.LocalAddr().String(), conn.peer.String(), "relay mode keeps talking to the relay agent")
}

func TestHandshakeErrors(t *testing.T) {
	srv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })
	mainServer = srv.LocalAddr().String()
	t.Cleanup(func() { mainServer = "www.easy4ipcloud.com:8800" })

	// the cloud does not know this serial
	go func() {
		buf := make([]byte, 4096)
		for {
			n, from, err := srv.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if strings.Contains(string(buf[:n]), "/online/p2psrv/") {
				_, _ = srv.WriteToUDP(xmlReply(404, "Not Found", ""), from)
			} else {
				_, _ = srv.WriteToUDP(xmlReply(200, "OK", ""), from)
			}
		}
	}()
	_, _, err = openSession("NOPE", false, nil)
	var re *ResponseError
	require.ErrorAs(t, err, &re)
	require.Equal(t, 404, re.Code)
}
