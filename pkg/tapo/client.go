package tapo

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
)

type Client struct {
	core.Listener

	url string

	medias    []*core.Media
	receivers []*core.Receiver
	sender    *core.Sender

	conn1 net.Conn
	conn2 net.Conn

	decrypt func(b []byte) []byte

	session1 string
	session2 string

	recv int
	send int
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
	c.conn1, err = c.newConn()
	return
}

func (c *Client) newConn() (net.Conn, error) {
	u, err := url.Parse(c.url)
	if err != nil {
		return nil, err
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

	// TODO: fix closing connection
	ctx, pconn := tcp.WithConn()
	req, err := http.NewRequestWithContext(ctx, "POST", u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "multipart/mixed; boundary=--client-stream-boundary--")

	res, err := tcp.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, errors.New(res.Status)
	}

	if c.decrypt == nil {
		c.newDectypter(res, username, password)
	}

	return *pconn, nil
}

func (c *Client) newDectypter(res *http.Response, username, password string) {
	// extract nonce from response
	// cipher="AES_128_CBC" username="admin" padding="PKCS7_16" algorithm="MD5" nonce="***"
	nonce := res.Header.Get("Key-Exchange")
	nonce = core.Between(nonce, `nonce="`, `"`)

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
}

func (c *Client) SetupStream() (err error) {
	if c.session1 != "" {
		return
	}

	// audio: default, disable, enable
	c.session1, err = c.Request(c.conn1, []byte(`{"params":{"preview":{"audio":["default"],"channels":[0],"resolutions":["HD"]},"method":"get"},"seq":1,"type":"request"}`))
	return
}

// Handle - first run will be in probe state
func (c *Client) Handle() error {
	rd := multipart.NewReader(c.conn1, "--device-stream-boundary--")
	demux := mpegts.NewDemuxer()

	for {
		p, err := rd.NextRawPart()
		if err != nil {
			return err
		}

		if ct := p.Header.Get("Content-Type"); ct != "video/mp2t" {
			continue
		}

		cl := p.Header.Get("Content-Length")
		size, err := strconv.Atoi(cl)
		if err != nil {
			return err
		}

		c.recv += size

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
		bytesRd := bytes.NewReader(body)

		for {
			pkt, err2 := demux.ReadPacket(bytesRd)
			if pkt == nil || err2 == io.EOF {
				break
			}
			if err2 != nil {
				return err2
			}

			for _, receiver := range c.receivers {
				if receiver.ID == pkt.PayloadType {
					mpegts.TimestampToRTP(pkt, receiver.Codec)
					receiver.WriteRTP(pkt)
					break
				}
			}
		}
	}
}

func (c *Client) Close() (err error) {
	if c.conn1 != nil {
		err = c.conn1.Close()
	}
	if c.conn2 != nil {
		_ = c.conn2.Close()
	}
	return
}

func (c *Client) Request(conn net.Conn, body []byte) (string, error) {
	// TODO: fixme (size)
	buf := bytes.NewBuffer(nil)
	buf.WriteString("----client-stream-boundary--\r\n")
	buf.WriteString("Content-Type: application/json\r\n")
	buf.WriteString("Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n")
	buf.Write(body)
	buf.WriteString("\r\n")

	if _, err := buf.WriteTo(conn); err != nil {
		return "", err
	}

	mpReader := multipart.NewReader(conn, "--device-stream-boundary--")

	for {
		p, err := mpReader.NextRawPart()
		if err != nil {
			return "", err
		}

		var v struct {
			Params struct {
				SessionID string `json:"session_id"`
			} `json:"params"`
		}

		if err = json.NewDecoder(p).Decode(&v); err != nil {
			return "", err
		}

		return v.Params.SessionID, nil
	}
}
