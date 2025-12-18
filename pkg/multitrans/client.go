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

	closed chan struct{}
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
		closed:     make(chan struct{}),
	}
	c.pass, _ = u.User.Password()

	fmt.Printf("[multitrans] Client %p created\n", c)

	if err = c.dial(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Client) Close() error {
	fmt.Printf("[multitrans] Client %p Close() called\n", c)

	select {
	case <-c.closed:
		return nil
	default:
		close(c.closed)
	}

	if c.sender != nil {
		c.sender.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	fmt.Printf("[multitrans] Client %p AddTrack kind=%s codec=%s direction=%s\n", c, media.Kind, track.Codec.Name, media.Direction)
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
			fmt.Printf("[multitrans] Client %p send RTP len=%d pt=%d seq=%d ts=%d header=%X\n",
				c, size, packet.PayloadType, packet.SequenceNumber, packet.Timestamp, b[:12])
		} else {
			fmt.Printf("[multitrans] Client %p send RTP len=%d (too short)\n", c, size)
		}
		// fmt.Printf("[multitrans] sending packet size=%d payload=%d\n", size, len(packet.Payload))

		header := make([]byte, 4)
		header[0] = '$'
		header[1] = 1 // Channel 1 for audio
		header[2] = byte(size >> 8)
		header[3] = byte(size)

		if _, err := c.conn.Write(header); err != nil {
			fmt.Printf("[multitrans] Client %p write header error: %v\n", c, err)
			return
		}
		if _, err := c.conn.Write(b); err != nil {
			fmt.Printf("[multitrans] Client %p write body error: %v\n", c, err)
			return
		}
	}

	c.sender.HandleRTP(track)
	return nil
}

func (c *Client) dial() error {
	fmt.Printf("[multitrans] Client %p dial() connecting to %s\n", c, c.URL.Host)
	conn, err := net.DialTimeout("tcp", c.URL.Host, time.Second*5)
	if err != nil {
		fmt.Printf("[multitrans] dial() tcp connection error: %v\n", err)
		return err
	}
	fmt.Printf("[multitrans] Client %p dial() tcp connected\n", c)

	c.conn = conn
	c.reader = bufio.NewReader(conn)

	// Handshake
	if err = c.handshake(); err != nil {
		fmt.Printf("[multitrans] dial() handshake error: %v\n", err)
		c.conn.Close()
		return err
	}

	fmt.Printf("[multitrans] Client %p dial() handshake success\n", c)
	return nil
}

// handshake ... (no change needed in signature, but internal logging could be updated, but Client %p is not easily passed unless we change method receiver to be logged? It is method receiver. I will leave it mostly as is but maybe add prefix if I edit it.)
// I will not edit handshake body just for logging to avoid large diff, unless necessary.
// Actually, I should probably update dial logs.

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
	fmt.Printf("[multitrans] Client %p handshake OK, session=%s\n", c, session)
	if session == "" {
		return errors.New("multitrans: no session")
	}

	return c.openTalkChannel(uri, session)
}

func (c *Client) openTalkChannel(uri, session string) error {
	payload := `{"type":"request","seq":0,"params":{"method":"get","talk":{"mode":"full_duplex"}}}`

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
		fmt.Printf("[multitrans] Client %p talkback error response: %s\n", c, string(res.Body))
		return fmt.Errorf("multitrans: talkback error: %s", string(res.Body))
	}
	fmt.Printf("[multitrans] Client %p talkback channel opened: %s\n", c, string(res.Body))

	return nil
}

func (c *Client) startTalkback() error {
	// Already connected and channel opened in Dial -> handshake -> openTalkChannel
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
	fmt.Printf("[multitrans] Client %p Start()\n", c)
	<-c.closed
	return nil
}

func (c *Client) Stop() error {
	fmt.Printf("[multitrans] Client %p Stop()\n", c)
	return c.Close()
}
