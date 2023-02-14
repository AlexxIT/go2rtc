package tapo

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"errors"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Client struct {
	streamer.Element

	url string

	medias []*streamer.Media
	tracks map[byte]*streamer.Track

	conn   net.Conn
	reader *multipart.Reader

	decrypt func(b []byte) []byte
}

// block ciphers using cipher block chaining.
type cbcMode interface {
	cipher.BlockMode
	SetIV([]byte)
}

func NewClient(url string) *Client {
	return &Client{url: url}
}

func (c *Client) Dial() (err error) {
	u, err := url.Parse(c.url)
	if err != nil {
		return
	}

	// support raw username/password
	username := u.User.Username()
	password, _ := u.User.Password()

	// or cloud password in place of username
	if password == "" {
		password = fmt.Sprintf("%16X", md5.Sum([]byte(username)))
		username = "admin"
		u.User = url.UserPassword(username, password)
	}

	u.Scheme = "http"
	u.Path = "/stream"
	if u.Port() == "" {
		u.Host += ":8800"
	}

	req, err := http.NewRequest("POST", u.String(), nil)
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "multipart/mixed; boundary=--client-stream-boundary--")

	res, err := tcp.Do(req)
	if err != nil {
		return
	}

	if res.StatusCode != http.StatusOK {
		return errors.New(res.Status)
	}

	// extract nonce from response
	// cipher="AES_128_CBC" username="admin" padding="PKCS7_16" algorithm="MD5" nonce="***"
	nonce := res.Header.Get("Key-Exchange")
	nonce = streamer.Between(nonce, `nonce="`, `"`)

	key := md5.Sum([]byte(nonce + ":" + password))
	iv := md5.Sum([]byte(username + ":" + nonce))

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return
	}

	cbc := cipher.NewCBCDecrypter(block, iv[:]).(cbcMode)

	c.decrypt = func(b []byte) []byte {
		// restore IV
		cbc.SetIV(iv[:])

		// decrypt
		cbc.CryptBlocks(b, b)

		// unpad
		padSize := int(b[len(b)-1])
		return b[:len(b)-padSize]
	}

	c.conn = res.Body.(net.Conn)

	boundary := res.Header.Get("Content-Type")
	_, boundary, _ = strings.Cut(boundary, "boundary=")

	c.reader = multipart.NewReader(c.conn, boundary)

	return nil
}

func (c *Client) Play() (err error) {
	// audio: default, disable, enable
	body := []byte(
		"----client-stream-boundary--\r\n" +
			"Content-Type: application/json\r\nContent-Length: 120\r\n\r\n" +
			`{"params":{"preview":{"audio":["default"],"channels":[0],"resolutions":["HD"]},"method":"get"},"seq":1,"type":"request"}` +
			"\r\n",
	)

	_, err = c.conn.Write(body)
	return nil
}

// Handle - first run will be in probe state
func (c *Client) Handle() error {
	if c.tracks == nil {
		c.tracks = map[byte]*streamer.Track{}
	}

	reader := mpegts.NewReader()

	probe := streamer.NewProbe(c.medias == nil)
	for probe == nil || probe.Active() {
		p, err := c.reader.NextRawPart()
		if err != nil {
			return err
		}

		ct := p.Header.Get("Content-Type")
		if ct != "video/mp2t" {
			continue
		}

		cl := p.Header.Get("Content-Length")

		size, err := strconv.Atoi(cl)
		if err != nil {
			return err
		}

		body := make([]byte, size)

		b := body
		for {
			if n, err2 := p.Read(b); err2 == nil {
				b = b[n:]
			} else {
				break
			}
		}

		body = c.decrypt(body)
		reader.SetBuffer(body)

		for {
			pkt := reader.GetPacket()
			if pkt == nil {
				break
			}

			track := c.tracks[pkt.PayloadType]
			if track == nil {
				// count track on probe state even if not support it
				probe.Append(pkt.PayloadType)

				media := mpegts.GetMedia(pkt)
				if media == nil {
					continue // unsupported codec
				}

				track = streamer.NewTrack2(media, nil)

				c.medias = append(c.medias, media)
				c.tracks[pkt.PayloadType] = track
			}

			_ = track.WriteRTP(pkt)
		}
	}

	return nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}
