package tapo

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
)

// Deprecated: should be rewritten to core.Connection
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
	request  string

	recv int
	send int
}

// block ciphers using cipher block chaining.
type cbcMode interface {
	cipher.BlockMode
	SetIV([]byte)
}

func Dial(url string) (*Client, error) {
	var err error
	c := &Client{url: url}
	if c.conn1, err = c.newConn(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) newConn() (net.Conn, error) {
	u, err := url.Parse(c.url)
	if err != nil {
		return nil, err
	}

	if u.Port() == "" {
		u.Host += ":8800"
	}

	req, err := http.NewRequest("POST", "http://"+u.Host+"/stream", nil)
	if err != nil {
		return nil, err
	}

	query := u.Query()

	if deviceId := query.Get("deviceId"); deviceId != "" {
		req.URL.RawQuery = "deviceId=" + deviceId
	}

	req.URL.User = u.User
	req.Header.Set("Content-Type", "multipart/mixed; boundary=--client-stream-boundary--")

	conn, res, err := dial(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, errors.New(res.Status)
	}

	if c.decrypt == nil {
		c.newDectypter(res)
	}

	channel := query.Get("channel")
	if channel == "" {
		channel = "0"
	}

	subtype := query.Get("subtype")
	switch subtype {
	case "", "0":
		subtype = "HD"
	case "1":
		subtype = "VGA"
	}

	c.request = fmt.Sprintf(
		`{"params":{"preview":{"audio":["default"],"channels":[%s],"resolutions":["%s"]},"method":"get"},"seq":1,"type":"request"}`,
		channel, subtype,
	)

	return conn, nil
}

func (c *Client) newDectypter(res *http.Response) {
	username := res.Request.URL.User.Username()
	password, _ := res.Request.URL.User.Password()

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
	c.session1, err = c.Request(c.conn1, []byte(c.request))
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

func dial(req *http.Request) (net.Conn, *http.Response, error) {
	conn, err := net.DialTimeout("tcp", req.URL.Host, core.ConnDialTimeout)
	if err != nil {
		return nil, nil, err
	}

	username := req.URL.User.Username()
	password, _ := req.URL.User.Password()
	req.URL.User = nil

	if err = req.Write(conn); err != nil {
		return nil, nil, err
	}

	r := bufio.NewReader(conn)

	res, err := http.ReadResponse(r, req)
	if err != nil {
		return nil, nil, err
	}
	_ = res.Body.Close() // ignore response body

	auth := res.Header.Get("WWW-Authenticate")

	if res.StatusCode != http.StatusUnauthorized || !strings.HasPrefix(auth, "Digest") {
		return nil, nil, fmt.Errorf("Expected StatusCode to be %d, received %d", http.StatusUnauthorized, res.StatusCode)
	}

	if password == "" {
		// support cloud password in place of username
		if strings.Contains(auth, `encrypt_type="3"`) {
			password = fmt.Sprintf("%32X", sha256.Sum256([]byte(username)))
		} else {
			password = fmt.Sprintf("%16X", md5.Sum([]byte(username)))
		}
		username = "admin"
	}

	realm := tcp.Between(auth, `realm="`, `"`)
	nonce := tcp.Between(auth, `nonce="`, `"`)
	qop := tcp.Between(auth, `qop="`, `"`)
	uri := req.URL.RequestURI()
	ha1 := tcp.HexMD5(username, realm, password)
	ha2 := tcp.HexMD5(req.Method, uri)
	nc := "00000001"
	cnonce := core.RandString(32, 64)
	response := tcp.HexMD5(ha1, nonce, nc, cnonce, qop, ha2)

	// https://datatracker.ietf.org/doc/html/rfc7616
	header := fmt.Sprintf(
		`Digest username="%s", realm="%s", nonce="%s", uri="%s", qop=%s, nc=%s, cnonce="%s", response="%s"`,
		username, realm, nonce, uri, qop, nc, cnonce, response,
	)

	if opaque := tcp.Between(auth, `opaque="`, `"`); opaque != "" {
		header += fmt.Sprintf(`, opaque="%s", algorithm=MD5`, opaque)
	}

	req.Header.Set("Authorization", header)

	if err = req.Write(conn); err != nil {
		return nil, nil, err
	}

	if res, err = http.ReadResponse(r, req); err != nil {
		return nil, nil, err
	}

	req.URL.User = url.UserPassword(username, password)

	return conn, res, nil
}
