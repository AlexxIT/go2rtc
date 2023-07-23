package hap

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/hap/chacha20poly1305"
	"github.com/AlexxIT/go2rtc/pkg/hap/curve25519"
	"github.com/AlexxIT/go2rtc/pkg/hap/ed25519"
	"github.com/AlexxIT/go2rtc/pkg/hap/hkdf"
	"github.com/AlexxIT/go2rtc/pkg/hap/secure"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
	"github.com/AlexxIT/go2rtc/pkg/mdns"
)

const (
	ConnDialTimeout = time.Second * 3
	ConnDeadline    = time.Second * 3
)

// Client for HomeKit. DevicePublic can be null.
type Client struct {
	DeviceAddress string // including port
	DeviceID      string // aka. Accessory
	DevicePublic  []byte
	ClientID      string // aka. Controller
	ClientPrivate []byte

	OnEvent func(res *http.Response)
	Output  func(msg any)

	conn   net.Conn
	reader *bufio.Reader
}

func NewClient(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	c := &Client{
		DeviceAddress: u.Host,
		DeviceID:      query.Get("device_id"),
		DevicePublic:  DecodeKey(query.Get("device_public")),
		ClientID:      query.Get("client_id"),
		ClientPrivate: DecodeKey(query.Get("client_private")),
	}

	return c, nil
}

func (c *Client) ClientPublic() []byte {
	return c.ClientPrivate[32:]
}

func (c *Client) URL() string {
	return fmt.Sprintf(
		"homekit://%s?device_id=%s&device_public=%16x&client_id=%s&client_private=%32x",
		c.DeviceAddress, c.DeviceID, c.DevicePublic, c.ClientID, c.ClientPrivate,
	)
}

func (c *Client) DeviceHost() string {
	if i := strings.IndexByte(c.DeviceAddress, ':'); i > 0 {
		return c.DeviceAddress[:i]
	}
	return c.DeviceAddress
}

func (c *Client) Dial() (err error) {
	// update device address (host and/or port) before dial
	_ = mdns.QueryOrDiscovery(c.DeviceHost(), mdns.ServiceHAP, func(entry *mdns.ServiceEntry) bool {
		if entry.Complete() && entry.Info["id"] == c.DeviceID {
			c.DeviceAddress = entry.Addr()
			return true
		}
		return false
	})

	if c.conn, err = net.DialTimeout("tcp", c.DeviceAddress, ConnDialTimeout); err != nil {
		return
	}

	c.reader = bufio.NewReader(c.conn)

	// STEP M1: send our session public to device
	sessionPublic, sessionPrivate := curve25519.GenerateKeyPair()

	// 1. Send sessionPublic
	plainM1 := struct {
		PublicKey []byte `tlv8:"3"`
		State     byte   `tlv8:"6"`
	}{
		PublicKey: sessionPublic,
		State:     StateM1,
	}
	res, err := c.Post(PathPairVerify, MimeTLV8, tlv8.MarshalReader(plainM1))
	if err != nil {
		return
	}

	// STEP M2: unpack deviceID from response
	var cipherM2 struct {
		PublicKey     []byte `tlv8:"3"`
		EncryptedData []byte `tlv8:"5"`
		State         byte   `tlv8:"6"`
	}
	if err = tlv8.UnmarshalReader(res.Body, &cipherM2); err != nil {
		return err
	}
	if cipherM2.State != StateM2 {
		return NewResponseError(plainM1, cipherM2)
	}

	// 1. generate session shared key
	sessionShared, err := curve25519.SharedSecret(sessionPrivate, cipherM2.PublicKey)
	if err != nil {
		return
	}

	sessionKey, err := hkdf.Sha512(
		sessionShared, "Pair-Verify-Encrypt-Salt", "Pair-Verify-Encrypt-Info",
	)
	if err != nil {
		return
	}

	// 2. decrypt M2 response with session key
	b, err := chacha20poly1305.Decrypt(sessionKey, "PV-Msg02", cipherM2.EncryptedData)
	if err != nil {
		return
	}

	// 3. unpack payload from TLV8
	var plainM2 struct {
		Identifier string `tlv8:"1"`
		Signature  []byte `tlv8:"10"`
	}
	if err = tlv8.Unmarshal(b, &plainM2); err != nil {
		return
	}

	// 4. verify signature for M2 response with device public
	// device session + device id + our session
	if c.DevicePublic != nil {
		b = Append(cipherM2.PublicKey, plainM2.Identifier, sessionPublic)
		if !ed25519.ValidateSignature(c.DevicePublic, b, plainM2.Signature) {
			return errors.New("hap: ValidateSignature")
		}
	}

	// STEP M3: send our clientID to device
	// 1. generate signature with our private key
	// (our session + our ID + device session)
	b = Append(sessionPublic, c.ClientID, cipherM2.PublicKey)
	if b, err = ed25519.Signature(c.ClientPrivate, b); err != nil {
		return
	}

	// 2. generate payload
	plainM3 := struct {
		Identifier string `tlv8:"1"`
		Signature  []byte `tlv8:"10"`
	}{
		Identifier: c.ClientID,
		Signature:  b,
	}
	if b, err = tlv8.Marshal(plainM3); err != nil {
		return
	}

	// 4. encrypt payload with session key
	if b, err = chacha20poly1305.Encrypt(sessionKey, "PV-Msg03", b); err != nil {
		return
	}

	// 4. generate request
	cipherM3 := struct {
		EncryptedData []byte `tlv8:"5"`
		State         byte   `tlv8:"6"`
	}{
		State:         StateM3,
		EncryptedData: b,
	}
	if res, err = c.Post(PathPairVerify, MimeTLV8, tlv8.MarshalReader(cipherM3)); err != nil {
		return
	}

	// STEP M4. Read response
	var plainM4 struct {
		State byte `tlv8:"6"`
	}
	if err = tlv8.UnmarshalReader(res.Body, &plainM4); err != nil {
		return
	}
	if plainM4.State != StateM4 {
		return NewResponseError(cipherM3, plainM4)
	}

	// like tls.Client wrapper over net.Conn
	if c.conn, err = secure.Client(c.conn, sessionShared, true); err != nil {
		return
	}
	// new reader for new conn
	c.reader = bufio.NewReaderSize(c.conn, 32*1024) // 32K like default request body

	return
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	conn := c.conn
	c.conn = nil
	return conn.Close()
}

func (c *Client) GetAccessories() ([]*Accessory, error) {
	res, err := c.Get(PathAccessories)
	if err != nil {
		return nil, err
	}

	var ac Accessories
	if err = json.NewDecoder(res.Body).Decode(&ac); err != nil {
		return nil, err
	}

	for _, accs := range ac.Accessories {
		for _, serv := range accs.Services {
			for _, char := range serv.Characters {
				char.AID = accs.AID
			}
		}
	}

	return ac.Accessories, nil
}

func (c *Client) GetCharacters(query string) ([]*Character, error) {
	res, err := c.Get(PathCharacteristics + "?id=" + query)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var ch Characters
	if err = json.Unmarshal(data, &ch); err != nil {
		return nil, err
	}
	return ch.Characters, nil
}

func (c *Client) GetCharacter(char *Character) error {
	query := fmt.Sprintf("%d.%d", char.AID, char.IID)
	chars, err := c.GetCharacters(query)
	if err != nil {
		return err
	}
	char.Value = chars[0].Value
	return nil
}

func (c *Client) PutCharacters(characters ...*Character) error {
	for i, char := range characters {
		if char.Event != nil {
			char = &Character{AID: char.AID, IID: char.IID, Event: char.Event}
		} else {
			char = &Character{AID: char.AID, IID: char.IID, Value: char.Value}
		}
		characters[i] = char
	}
	data, err := json.Marshal(Characters{characters})
	if err != nil {
		return err
	}

	_, err = c.Put(PathCharacteristics, MimeJSON, bytes.NewReader(data))
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) GetImage(width, height int) ([]byte, error) {
	s := fmt.Sprintf(
		`{"image-width":%d,"image-height":%d,"resource-type":"image","reason":0}`,
		width, height,
	)
	res, err := c.Post(PathResource, MimeJSON, bytes.NewBufferString(s))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(res.Body)
}

func (c *Client) LocalAddr() string {
	return c.conn.LocalAddr().String()
}

func DecodeKey(s string) []byte {
	if s == "" {
		return nil
	}
	data, err := hex.DecodeString(s)
	if err != nil {
		return nil
	}
	return data
}
