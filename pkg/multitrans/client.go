package multitrans

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/google/uuid"
	"github.com/pion/rtp"
)

type Client struct {
	core.Connection
	conn   net.Conn
	rd     *bufio.Reader
	closed core.Waiter
}

func Dial(rawURL string) (core.Producer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	if u.Port() == "" {
		u.Host += ":554"
	}

	conn, err := net.DialTimeout("tcp", u.Host, core.ConnDialTimeout)
	if err != nil {
		return nil, err
	}

	c := &Client{
		conn: conn,
		rd:   bufio.NewReader(conn),
	}

	if err = c.handshake(u); err != nil {
		_ = conn.Close()
		return nil, err
	}

	c.Connection = core.Connection{
		ID:         core.NewID(),
		FormatName: "multitrans",
		Protocol:   "rtsp",
		RemoteAddr: conn.RemoteAddr().String(),
		Source:     rawURL,
		Medias: []*core.Media{
			{
				Kind:      core.KindAudio,
				Direction: core.DirectionSendonly,
				Codecs:    []*core.Codec{{Name: core.CodecPCMA, ClockRate: 8000, PayloadType: 8}},
			},
		},
		Transport: conn,
	}

	return c, nil
}

func (c *Client) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	sender := core.NewSender(media, track.Codec)
	sender.Handler = func(packet *rtp.Packet) {
		clone := rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         packet.Marker,
				PayloadType:    8,
				SequenceNumber: packet.SequenceNumber,
				Timestamp:      packet.Timestamp,
				SSRC:           packet.SSRC,
			},
			Payload: packet.Payload,
		}

		// Encapsulate in RTSP Interleaved Frame (Channel 1)
		// $ + Channel(1 byte) + Length(2 bytes) + Packet
		size := 12 + len(clone.Payload)
		b := make([]byte, 4+size)
		b[0] = '$'
		b[1] = 1 // Channel 1 for audio
		b[2] = byte(size >> 8)
		b[3] = byte(size)
		if _, err := clone.MarshalTo(b[4:]); err != nil {
			return
		}
		if _, err := c.conn.Write(b); err != nil {
			return
		}
	}
	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}

func (c *Client) handshake(u *url.URL) error {
	// Step 1: Get Challenge
	uid := uuid.New().String()

	uri := fmt.Sprintf("rtsp://%s/multitrans", u.Host)
	data := fmt.Sprintf("MULTITRANS %s RTSP/1.0\r\nCSeq: 0\r\nX-Client-UUID: %s\r\n\r\n", uri, uid)

	if _, err := c.conn.Write([]byte(data)); err != nil {
		return err
	}

	res, err := tcp.ReadResponse(c.rd)
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
	user := u.User.Username()
	pass, _ := u.User.Password()

	ha1 := tcp.HexMD5(user, realm, pass)
	ha2 := tcp.HexMD5("MULTITRANS", uri)
	response := tcp.HexMD5(ha1, nonce, ha2)

	authHeader := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`,
		user, realm, nonce, uri, response)

	data = fmt.Sprintf("MULTITRANS %s RTSP/1.0\r\nCSeq: 1\r\nAuthorization: %s\r\nX-Client-UUID: %s\r\n\r\n",
		uri, authHeader, uid)

	if _, err = c.conn.Write([]byte(data)); err != nil {
		return err
	}

	res, err = tcp.ReadResponse(c.rd)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		return errors.New("multitrans: auth failed: " + res.Status)
	}

	// Session: 7116520596809429228
	session := res.Header.Get("Session")
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

	res, err := tcp.ReadResponse(c.rd)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		return errors.New("multitrans: talkback failed: " + res.Status)
	}

	// Python checks for "error_code":0 in body.
	if !bytes.Contains(res.Body, []byte(`"error_code":0`)) {
		return fmt.Errorf("multitrans: talkback error: %s", string(res.Body))
	}

	return nil
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

func (c *Client) Start() error {
	_ = c.closed.Wait()
	return nil
}

func (c *Client) Stop() error {
	c.closed.Done(nil)
	return c.Connection.Stop()
}
