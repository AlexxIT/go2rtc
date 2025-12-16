package multitrans

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/google/uuid"
	"github.com/pion/rtp"
)

type Client struct {
	core.Listener

	URL *url.URL

	conn       net.Conn
	reader     *bufio.Reader
	sender     *core.Sender
	clientUUID string

	user string
	pass string
}

func Dial(rawURL string) (*Client, error) {
	fmt.Printf("[multitrans] Dial called with URL: %s\n", rawURL)
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	if u.Port() == "" {
		u.Host += ":554"
	}

	c := &Client{
		URL:        u,
		user:       u.User.Username(),
		clientUUID: uuid.New().String(),
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
	fmt.Printf("[multitrans] AddTrack kind=%s codec=%s direction=%s\n", media.Kind, track.Codec.Name, media.Direction)
	if c.sender != nil {
		return errors.New("multitrans: sender already exists")
	}

	if track.Codec.Name != core.CodecPCMA {
		fmt.Printf("[multitrans] unsupported codec: %s\n", track.Codec.Name)
		return errors.New("multitrans: only PCMA supported")
	}

	if err := c.startTalkback(); err != nil {
		return err
	}

	c.sender = core.NewSender(media, track.Codec)
	c.sender.Handler = func(packet *rtp.Packet) {
		// Encapsulate in RTSP Interleaved Frame (Channel 1)
		// $ + Channel(1 byte) + Length(2 bytes) + Packet
		b, err := packet.Marshal()
		if err != nil {
			return
		}

		size := len(b)
		// Log RTP header (first 12 bytes)
		if size >= 12 {
			fmt.Printf("[multitrans] send RTP len=%d pt=%d seq=%d ts=%d header=%X\n",
				size, packet.PayloadType, packet.SequenceNumber, packet.Timestamp, b[:12])
		} else {
			fmt.Printf("[multitrans] send RTP len=%d (too short)\n", size)
		}
		// fmt.Printf("[multitrans] sending packet size=%d payload=%d\n", size, len(packet.Payload))

		header := make([]byte, 4)
		header[0] = '$'
		header[1] = 1 // Channel 1 for audio
		header[2] = byte(size >> 8)
		header[3] = byte(size)

		if _, err := c.conn.Write(header); err != nil {
			fmt.Printf("[multitrans] write header error: %v\n", err)
			return
		}
		if _, err := c.conn.Write(b); err != nil {
			fmt.Printf("[multitrans] write body error: %v\n", err)
			return
		}
	}

	c.sender.HandleRTP(track)
	return nil
}

func (c *Client) dial() error {
	fmt.Printf("[multitrans] dial() connecting to %s\n", c.URL.Host)
	conn, err := net.DialTimeout("tcp", c.URL.Host, time.Second*5)
	if err != nil {
		fmt.Printf("[multitrans] dial() tcp connection error: %v\n", err)
		return err
	}
	fmt.Printf("[multitrans] dial() tcp connected\n")

	c.conn = conn
	c.reader = bufio.NewReader(conn)

	// Handshake
	if err = c.handshake(); err != nil {
		fmt.Printf("[multitrans] dial() handshake error: %v\n", err)
		c.conn.Close()
		return err
	}

	fmt.Printf("[multitrans] dial() handshake success\n")
	return nil
}

func (c *Client) handshake() error {
	// Step 1: Get Challenge
	uri := fmt.Sprintf("rtsp://%s/multitrans", c.URL.Host)
	data := fmt.Sprintf("MULTITRANS %s RTSP/1.0\r\nCSeq: 0\r\nX-Client-UUID: %s\r\n\r\n", uri, c.clientUUID)

	if _, err := c.conn.Write([]byte(data)); err != nil {
		return err
	}

	res, err := tcp.ReadResponse(c.reader)
	if err != nil {
		return err
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
		uri, authHeader, c.clientUUID)

	if _, err := c.conn.Write([]byte(data)); err != nil {
		return err
	}

	res, err = tcp.ReadResponse(c.reader)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		return errors.New("multitrans: auth failed: " + res.Status)
	}

	// Session: 7116520596809429228
	session := res.Header.Get("Session")
	fmt.Printf("[multitrans] handshake OK, session=%s\n", session)
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

	res, err := tcp.ReadResponse(c.reader)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		return errors.New("multitrans: talkback failed: " + res.Status)
	}

	// Python checks for "error_code":0 in body.
	if !bytes.Contains(res.Body, []byte(`"error_code":0`)) {
		fmt.Printf("[multitrans] talkback error response: %s\n", string(res.Body))
		return fmt.Errorf("multitrans: talkback error: %s", string(res.Body))
	}
	fmt.Printf("[multitrans] talkback channel opened: %s\n", string(res.Body))

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

func (c *Client) GetMedias() []*core.Media {
	return []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs:    []*core.Codec{{Name: core.CodecPCMA, ClockRate: 8000, PayloadType: 8}},
		},
	}
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return nil, errors.New("multitrans: not supported")
}

func (c *Client) Start() error {
	return nil
}

func (c *Client) Stop() error {
	return c.Close()
}
