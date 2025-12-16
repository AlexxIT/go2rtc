package multitrans

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtp"
)

type Client struct {
	core.Listener

	URL *url.URL

	conn   net.Conn
	reader *bufio.Reader
	sender *core.Sender

	user string
	pass string
}

func Dial(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	if u.Port() == "" {
		u.Host += ":554"
	}

	c := &Client{
		URL:  u,
		user: u.User.Username(),
	}
	c.pass, _ = u.User.Password()

	if err = c.dial(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Client) Close() error {
	if c.sender != nil {
		c.sender.Close()
	}
	return c.conn.Close()
}

func (c *Client) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	if c.sender != nil {
		return errors.New("multitrans: sender already exists")
	}

	if track.Codec.Name != core.CodecPCMA {
		return errors.New("multitrans: only PCMA supported")
	}

	if err := c.startTalkback(); err != nil {
		return err
	}

	c.sender = core.NewSender(media, track.Codec)
	c.sender.Handler = func(packet *rtp.Packet) {
		// Encapsulate in RTSP Interleaved Frame (Channel 1)
		// $ + Channel(1 byte) + Length(2 bytes) + Payload
		size := len(packet.Payload)
		b := make([]byte, 4+size)
		b[0] = '$'
		b[1] = 1 // Channel 1 for audio
		b[2] = byte(size >> 8)
		b[3] = byte(size)
		copy(b[4:], packet.Payload)

		if _, err := c.conn.Write(b); err != nil {
			// stop handler on error?
		}
	}

	c.sender.HandleRTP(track)
	return nil
}

func (c *Client) dial() error {
	conn, err := tcp.Dial(c.URL, time.Second*5)
	if err != nil {
		return err
	}

	c.conn = conn
	c.reader = bufio.NewReader(conn)

	// Handshake
	if err = c.handshake(); err != nil {
		c.conn.Close()
		return err
	}

	return nil
}

func (c *Client) handshake() error {
	// Step 1: Get Challenge
	uri := fmt.Sprintf("rtsp://%s/multitrans", c.URL.Host)
	data := fmt.Sprintf("MULTITRANS %s RTSP/1.0\r\nCSeq: 0\r\nX-Client-UUID: %s\r\n\r\n", uri, core.NewID())
	
	if _, err := c.conn.Write([]byte(data)); err != nil {
		return err
	}

	res, err := http.ReadResponse(c.reader, nil)
	if err != nil {
		return err
	}
	// Consume body if any (should be empty for 401)
	if res.Body != nil {
		res.Body.Close()
	}

	if res.StatusCode != http.StatusUnauthorized {
		return errors.New("multitrans: expected 401, got " + res.Status)
	}

	auth := res.Header.Get("WWW-Authenticate")
	realm := tcp.Between(auth, `realm="`, `"`)
	nonce := tcp.Between(auth, `nonce="`, `"`)

	// Step 2: Send Auth
	ha1 := tcp.HexMD5(c.user, realm, c.pass)
	ha2 := tcp.HexMD5("MULTITRANS", uri)
	response := tcp.HexMD5(ha1, nonce, ha2)

	authHeader := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`,
		c.user, realm, nonce, uri, response)

	data = fmt.Sprintf("MULTITRANS %s RTSP/1.0\r\nCSeq: 1\r\nAuthorization: %s\r\nX-Client-UUID: %s\r\n\r\n", 
		uri, authHeader, core.NewID())

	if _, err := c.conn.Write([]byte(data)); err != nil {
		return err
	}

	res, err = http.ReadResponse(c.reader, nil)
	if err != nil {
		return err
	}
	if res.Body != nil {
		res.Body.Close()
	}

	if res.StatusCode != http.StatusOK {
		return errors.New("multitrans: auth failed: " + res.Status)
	}

	// Session: 7116520596809429228
	session := res.Header.Get("Session")
	if session == "" {
		return errors.New("multitrans: no session")
	}
	
	// Store cookie/session for next request if needed, but here we just need it for startTalkback
	// Actually talkback uses the same conn, so we can store it in Client if we want, or just pass it.
	// We'll store it in Client struct to be safe.
	// But wait, the python script uses `session_id` from step 2 in step 3.
	// And the python script sends step 3 in `_connect_and_auth`.
	// So `Dial` should probably complete all 3 steps?
	// The python script `_connect_and_auth` does all 3 steps and returns the socket.
	// So yes, we should do step 3 here as well.
	
	return c.openTalkChannel(uri, session)
}

func (c *Client) openTalkChannel(uri, session string) error {
	payload := `{"type":"request","seq":0,"params":{"method":"get","talk":{"mode":"half_duplex"}}}`
	
	data := fmt.Sprintf("MULTITRANS %s RTSP/1.0\r\nCSeq: 2\r\nSession: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s",
		uri, session, len(payload), payload)

	if _, err := c.conn.Write([]byte(data)); err != nil {
		return err
	}

	res, err := http.ReadResponse(c.reader, nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return errors.New("multitrans: talkback failed: " + res.Status)
	}

	// Python checks for "error_code":0 in body.
	// Read body
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(res.Body); err != nil {
		return err
	}

	if !bytes.Contains(buf.Bytes(), []byte(`"error_code":0`)) {
		return fmt.Errorf("multitrans: talkback error: %s", buf.String())
	}

	return nil
}

func (c *Client) startTalkback() error {
	// Already connected and channel opened in Dial -> handshake -> openTalkChannel
	// So we just verify connection is good?
	if c.conn == nil {
		return errors.New("multitrans: not connected")
	}
	return nil
}
