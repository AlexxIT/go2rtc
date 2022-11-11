package hap

import (
	"bufio"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/hap/mdns"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/brutella/hap"
	"github.com/brutella/hap/chacha20poly1305"
	"github.com/brutella/hap/curve25519"
	"github.com/brutella/hap/ed25519"
	"github.com/brutella/hap/hkdf"
	"github.com/brutella/hap/tlv8"
	"github.com/tadglines/go-pkgs/crypto/srp"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Conn for HomeKit. DevicePublic can be null.
type Conn struct {
	streamer.Element

	DeviceAddress string // including port
	DeviceID      string
	DevicePublic  []byte
	ClientID      string
	ClientPrivate []byte

	OnEvent func(res *http.Response)
	Output  func(msg interface{})

	conn         net.Conn
	secure       *Secure
	httpResponse chan *bufio.Reader
}

func NewConn(rawURL string) (*Conn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	c := &Conn{
		DeviceAddress: u.Host,
		DeviceID:      query.Get("device_id"),
		DevicePublic:  DecodeKey(query.Get("device_public")),
		ClientID:      query.Get("client_id"),
		ClientPrivate: DecodeKey(query.Get("client_private")),
	}

	return c, nil
}

func Pair(deviceID, pin string) (*Conn, error) {
	entry := mdns.GetEntry(deviceID)
	if entry == nil {
		return nil, errors.New("can't find device via mDNS")
	}

	c := &Conn{
		DeviceAddress: fmt.Sprintf("%s:%d", entry.AddrV4.String(), entry.Port),
		DeviceID:      deviceID,
		ClientID:      GenerateUUID(),
		ClientPrivate: GenerateKey(),
	}

	var mfi bool
	for _, field := range entry.InfoFields {
		if field[:2] == "ff" {
			if field[3] == '1' {
				mfi = true
			}
			break
		}
	}

	return c, c.Pair(mfi, pin)
}

func (c *Conn) ClientPublic() []byte {
	return c.ClientPrivate[32:]
}

func (c *Conn) URL() string {
	return fmt.Sprintf(
		"homekit://%s?device_id=%s&device_public=%16x&client_id=%s&client_private=%32x",
		c.DeviceAddress, c.DeviceID, c.DevicePublic, c.ClientID, c.ClientPrivate,
	)
}

func (c *Conn) DialAndServe() error {
	if err := c.Dial(); err != nil {
		return err
	}
	return c.Handle()
}

func (c *Conn) Dial() error {
	// update device host before dial
	if host := mdns.GetAddress(c.DeviceID); host != "" {
		c.DeviceAddress = host
	}

	var err error
	c.conn, err = net.DialTimeout("tcp", c.DeviceAddress, time.Second*5)
	if err != nil {
		return err
	}

	// STEP M1: send our session public to device
	sessionPublic, sessionPrivate := curve25519.GenerateKeyPair()

	// 1. generate payload
	// important not include other fields
	requestM1 := struct {
		State     byte   `tlv8:"6"`
		PublicKey []byte `tlv8:"3"`
	}{
		State:     hap.M1,
		PublicKey: sessionPublic[:],
	}
	// 2. pack payload to TLV8
	buf, err := tlv8.Marshal(requestM1)
	if err != nil {
		return err
	}

	// 3. send request
	resp, err := c.Post(UriPairVerify, buf)
	if err != nil {
		return err
	}

	// STEP M2: unpack deviceID from response
	responseM2 := PairVerifyPayload{}
	if err = tlv8.UnmarshalReader(resp.Body, &responseM2); err != nil {
		return err
	}

	// 1. generate session shared key
	var deviceSessionPublic [32]byte
	copy(deviceSessionPublic[:], responseM2.PublicKey)
	sessionShared := curve25519.SharedSecret(sessionPrivate, deviceSessionPublic)
	sessionKey, err := hkdf.Sha512(
		sessionShared[:], []byte("Pair-Verify-Encrypt-Salt"),
		[]byte("Pair-Verify-Encrypt-Info"),
	)

	// 2. decrypt M2 response with session key
	msg := responseM2.EncryptedData[:len(responseM2.EncryptedData)-16]
	var mac [16]byte
	copy(mac[:], responseM2.EncryptedData[len(msg):]) // 16 byte (MAC)

	buf, err = chacha20poly1305.DecryptAndVerify(
		sessionKey[:], []byte("PV-Msg02"), msg, mac, nil,
	)

	// 3. unpack payload from TLV8
	payloadM2 := PairVerifyPayload{}
	if err = tlv8.Unmarshal(buf, &payloadM2); err != nil {
		return err
	}

	// 4. verify signature for M2 response with device public
	// device session + device id + our session
	if c.DevicePublic != nil {
		buf = nil
		buf = append(buf, responseM2.PublicKey[:]...)
		buf = append(buf, []byte(payloadM2.Identifier)...)
		buf = append(buf, sessionPublic[:]...)
		if !ed25519.ValidateSignature(
			c.DevicePublic[:], buf, payloadM2.Signature,
		) {
			return errors.New("device public signature invalid")
		}
	}

	// STEP M3: send our clientID to device
	// 1. generate signature with our private key
	// (our session + our ID + device session)
	buf = nil
	buf = append(buf, sessionPublic[:]...)
	buf = append(buf, []byte(c.ClientID)...)
	buf = append(buf, responseM2.PublicKey[:]...)
	signature, err := ed25519.Signature(c.ClientPrivate[:], buf)
	if err != nil {
		return err
	}

	// 2. generate payload
	payloadM3 := struct {
		Identifier string `tlv8:"1"`
		Signature  []byte `tlv8:"10"`
	}{
		Identifier: c.ClientID,
		Signature:  signature,
	}
	// 3. pack payload to TLV8
	buf, err = tlv8.Marshal(payloadM3)
	if err != nil {
		return err
	}

	// 4. encrypt payload with session key
	msg, mac, _ = chacha20poly1305.EncryptAndSeal(
		sessionKey[:], []byte("PV-Msg03"), buf, nil,
	)

	// 4. generate request
	requestM3 := struct {
		EncryptedData []byte `tlv8:"5"`
		State         byte   `tlv8:"6"`
	}{
		State:         hap.M3,
		EncryptedData: append(msg, mac[:]...),
	}
	// 5. pack payload to TLV8
	buf, err = tlv8.Marshal(requestM3)
	if err != nil {
		return err
	}

	resp, err = c.Post(UriPairVerify, buf)
	if err != nil {
		return err
	}

	// STEP M4. Read response
	responseM4 := PairVerifyPayload{}
	if err = tlv8.UnmarshalReader(resp.Body, &responseM4); err != nil {
		return err
	}

	// 1. check response state
	if responseM4.State != 4 || responseM4.Status != 0 {
		return fmt.Errorf("wrong M4 response: %+v", responseM4)
	}

	c.secure, err = NewSecure(sessionShared, false)
	//c.secure.Buffer = bytes.NewBuffer(nil)
	c.secure.Conn = c.conn

	c.httpResponse = make(chan *bufio.Reader, 10)

	return err
}

// https://github.com/apple/HomeKitADK/blob/master/HAP/HAPPairingPairSetup.c
func (c *Conn) Pair(mfi bool, pin string) (err error) {
	pin = strings.ReplaceAll(pin, "-", "")
	if len(pin) != 8 {
		return fmt.Errorf("wrong PIN format: %s", pin)
	}
	pin = pin[:3] + "-" + pin[3:5] + "-" + pin[5:]

	c.conn, err = net.Dial("tcp", c.DeviceAddress)
	if err != nil {
		return
	}

	// STEP M1. Generate request
	reqM1 := struct {
		Method byte `tlv8:"0"`
		State  byte `tlv8:"6"`
	}{
		State: hap.M1,
	}
	if mfi {
		reqM1.Method = 1 // ff=1 => method=1, ff=2 => method=0
	}
	buf, err := tlv8.Marshal(reqM1)
	if err != nil {
		return
	}

	// STEP M1. Send request
	res, err := c.Post(UriPairSetup, buf)
	if err != nil {
		return
	}

	// STEP M2. Read response
	resM2 := struct {
		Salt      []byte `tlv8:"2"`
		PublicKey []byte `tlv8:"3"` // server public key, aka session.B
		State     byte   `tlv8:"6"`
		Error     byte   `tlv8:"7"`
	}{}
	if err = tlv8.UnmarshalReader(res.Body, &resM2); err != nil {
		return
	}
	if resM2.State != 2 || resM2.Error > 0 {
		return fmt.Errorf("wrong M2: %+v", resM2)
	}

	// STEP M3. Generate session using pin
	username := []byte("Pair-Setup")

	SRP, err := srp.NewSRP(
		"rfc5054.3072", sha512.New, keyDerivativeFuncRFC2945(username),
	)
	if err != nil {
		return
	}

	SRP.SaltLength = 16

	// username: "Pair-Setup"
	// password: PIN (with dashes)
	session := SRP.NewClientSession(username, []byte(pin))
	sessionShared, err := session.ComputeKey(resM2.Salt, resM2.PublicKey)
	if err != nil {
		return
	}

	// STEP M3. Generate request
	reqM3 := struct {
		PublicKey []byte `tlv8:"3"`
		Proof     []byte `tlv8:"4"`
		State     byte   `tlv8:"6"`
	}{
		PublicKey: session.GetA(), // client public key, aka session.A
		Proof:     session.ComputeAuthenticator(),
		State:     hap.M3,
	}
	buf, err = tlv8.Marshal(reqM3)
	if err != nil {
		return err
	}

	// STEP M3. Send request
	res, err = c.Post(UriPairSetup, buf)
	if err != nil {
		return
	}

	// STEP M4. Read response
	resM4 := struct {
		Proof []byte `tlv8:"4"` // server proof
		State byte   `tlv8:"6"`
		Error byte   `tlv8:"7"`
	}{}
	if err = tlv8.UnmarshalReader(res.Body, &resM4); err != nil {
		return
	}
	if resM4.Error == 2 {
		return fmt.Errorf("wrong PIN: %s", pin)
	}
	if resM4.State != 4 || resM4.Error > 0 {
		return fmt.Errorf("wrong M4: %+v", resM4)
	}

	// STEP M4. Verify response
	if !session.VerifyServerAuthenticator(resM4.Proof) {
		return errors.New("verify server auth fail")
	}

	// STEP M5. Generate signature
	saltKey, err := hkdf.Sha512(
		sessionShared, []byte("Pair-Setup-Controller-Sign-Salt"),
		[]byte("Pair-Setup-Controller-Sign-Info"),
	)
	if err != nil {
		return
	}

	buf = nil
	buf = append(buf, saltKey[:]...)
	buf = append(buf, []byte(c.ClientID)...)
	buf = append(buf, c.ClientPublic()...)

	signature, err := ed25519.Signature(c.ClientPrivate, buf)
	if err != nil {
		return
	}

	// STEP M5. Generate payload
	msgM5 := struct {
		Identifier string `tlv8:"1"`
		PublicKey  []byte `tlv8:"3"`
		Signature  []byte `tlv8:"10"`
	}{
		Identifier: c.ClientID,
		PublicKey:  c.ClientPublic(),
		Signature:  signature,
	}
	buf, err = tlv8.Marshal(msgM5)
	if err != nil {
		return
	}

	// STEP M5. Encrypt payload
	sessionKey, err := hkdf.Sha512(
		sessionShared, []byte("Pair-Setup-Encrypt-Salt"),
		[]byte("Pair-Setup-Encrypt-Info"),
	)
	if err != nil {
		return
	}
	buf, mac, _ := chacha20poly1305.EncryptAndSeal(
		sessionKey[:], []byte("PS-Msg05"), buf, nil,
	)

	// STEP M5. Generate request
	reqM5 := struct {
		EncryptedData []byte `tlv8:"5"`
		State         byte   `tlv8:"6"`
	}{
		EncryptedData: append(buf, mac[:]...),
		State:         hap.M5,
	}
	buf, err = tlv8.Marshal(reqM5)
	if err != nil {
		return err
	}

	// STEP M5. Send request
	res, err = c.Post(UriPairSetup, buf)
	if err != nil {
		return
	}

	// STEP M6. Read response
	resM6 := struct {
		EncryptedData []byte `tlv8:"5"`
		State         byte   `tlv8:"6"`
		Error         byte   `tlv8:"7"`
	}{}
	if err = tlv8.UnmarshalReader(res.Body, &resM6); err != nil {
		return
	}
	if resM6.State != 6 || resM6.Error > 0 {
		return fmt.Errorf("wrong M6: %+v", resM2)
	}

	// STEP M6. Decrypt payload
	msg := resM6.EncryptedData[:len(resM6.EncryptedData)-16]
	copy(mac[:], resM6.EncryptedData[len(msg):]) // 16 byte (MAC)

	buf, err = chacha20poly1305.DecryptAndVerify(
		sessionKey[:], []byte("PS-Msg06"), msg, mac, nil,
	)
	if err != nil {
		return
	}

	msgM6 := struct {
		Identifier []byte `tlv8:"1"`
		PublicKey  []byte `tlv8:"3"`
		Signature  []byte `tlv8:"10"`
	}{}
	if err = tlv8.Unmarshal(buf, &msgM6); err != nil {
		return
	}

	// STEP M6. Verify payload
	if saltKey, err = hkdf.Sha512(
		sessionShared, []byte("Pair-Setup-Accessory-Sign-Salt"),
		[]byte("Pair-Setup-Accessory-Sign-Info"),
	); err != nil {
		return
	}

	buf = nil
	buf = append(buf, saltKey[:]...)
	buf = append(buf, msgM6.Identifier...)
	buf = append(buf, msgM6.PublicKey...)

	if !ed25519.ValidateSignature(
		msgM6.PublicKey[:], buf, msgM6.Signature,
	) {
		return errors.New("wrong server signature")
	}

	if c.DeviceID != string(msgM6.Identifier) {
		return fmt.Errorf("wrong Device ID: %s", msgM6.Identifier)
	}

	c.DevicePublic = msgM6.PublicKey

	return nil
}

func (c *Conn) Close() error {
	if c.conn == nil {
		return nil
	}
	conn := c.conn
	c.conn = nil
	return conn.Close()
}

func (c *Conn) GetAccessories() ([]*Accessory, error) {
	res, err := c.Get("/accessories")
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	p := Accessories{}
	if err = json.Unmarshal(data, &p); err != nil {
		return nil, err
	}

	for _, accs := range p.Accessories {
		for _, serv := range accs.Services {
			for _, char := range serv.Characters {
				char.AID = accs.AID
			}
		}
	}

	return p.Accessories, nil
}

func (c *Conn) GetCharacters(query string) ([]*Character, error) {
	res, err := c.Get("/characteristics?id=" + query)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	ch := Characters{}
	if err = json.Unmarshal(data, &ch); err != nil {
		return nil, err
	}
	return ch.Characters, nil
}

func (c *Conn) GetCharacter(char *Character) error {
	query := fmt.Sprintf("%d.%d", char.AID, char.IID)
	chars, err := c.GetCharacters(query)
	if err != nil {
		return err
	}
	char.Value = chars[0].Value
	return nil
}

func (c *Conn) PutCharacters(characters ...*Character) (err error) {
	for i, char := range characters {
		if char.Event != nil {
			char = &Character{AID: char.AID, IID: char.IID, Event: char.Event}
		} else {
			char = &Character{AID: char.AID, IID: char.IID, Value: char.Value}
		}
		characters[i] = char
	}
	var data []byte
	if data, err = json.Marshal(Characters{characters}); err != nil {
		return
	}

	var res *http.Response
	if res, err = c.Put("/characteristics", data); err != nil {
		return
	}

	if res.StatusCode >= 400 {
		return errors.New("wrong response status")
	}

	return
}

func (c *Conn) GetImage(width, height int) ([]byte, error) {
	res, err := c.Post(
		"/resource", []byte(fmt.Sprintf(
			`{"image-width":%d,"image-height":%d,"resource-type":"image","reason":0}`,
			width, height,
		)),
	)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(res.Body)
}

//func (c *Client) onEventData(r io.Reader) error {
//	if c.OnEvent == nil {
//		return nil
//	}
//
//	data, err := io.ReadAll(r)
//
//	ch := Characters{}
//	if err = json.Unmarshal(data, &ch); err != nil {
//		return err
//	}
//
//	c.OnEvent(ch.Characters)
//
//	return nil
//}

func (c *Conn) ListPairings() error {
	pReq := struct {
		Method byte `tlv8:"0"`
		State  byte `tlv8:"6"`
	}{
		Method: hap.MethodListPairings,
		State:  hap.M1,
	}
	data, err := tlv8.Marshal(pReq)
	if err != nil {
		return err
	}

	res, err := c.Post("/pairings", data)
	if err != nil {
		return err
	}

	data, err = io.ReadAll(res.Body)
	// TODO: don't know how to fix array of items
	var pRes struct {
		State      byte   `tlv8:"6"`
		Identifier string `tlv8:"1"`
		PublicKey  []byte `tlv8:"3"`
		Permission byte   `tlv8:"11"`
	}
	if err = tlv8.Unmarshal(data, &pRes); err != nil {
		return err
	}

	return nil
}

func (c *Conn) PairingsAdd(clientID string, clientPublic []byte, admin bool) error {
	pReq := struct {
		Method     byte   `tlv8:"0"`
		Identifier string `tlv8:"1"`
		PublicKey  []byte `tlv8:"3"`
		State      byte   `tlv8:"6"`
		Permission byte   `tlv8:"11"`
	}{
		Method:     hap.MethodAddPairing,
		Identifier: clientID,
		PublicKey:  clientPublic,
		State:      hap.M1,
		Permission: hap.PermissionUser,
	}
	if admin {
		pReq.Permission = hap.PermissionAdmin
	}

	data, err := tlv8.Marshal(pReq)
	if err != nil {
		return err
	}

	res, err := c.Post("/pairings", data)
	if err != nil {
		return err
	}

	data, err = io.ReadAll(res.Body)
	var pRes struct {
		State   byte `tlv8:"6"`
		Unknown byte `tlv8:"7"`
	}
	if err = tlv8.Unmarshal(data, &pRes); err != nil {
		return err
	}

	return nil
}

func (c *Conn) DeletePairing(id string) error {
	reqM1 := struct {
		State      byte   `tlv8:"6"`
		Method     byte   `tlv8:"0"`
		Identifier string `tlv8:"1"`
	}{
		State:      hap.M1,
		Method:     hap.MethodDeletePairing,
		Identifier: id,
	}
	data, err := tlv8.Marshal(reqM1)
	if err != nil {
		return err
	}

	res, err := c.Post("/pairings", data)
	if err != nil {
		return err
	}

	data, err = io.ReadAll(res.Body)
	var resM2 struct {
		State byte `tlv8:"6"`
	}
	if err = tlv8.Unmarshal(data, &resM2); err != nil {
		return err
	}
	if resM2.State != hap.M2 {
		return errors.New("wrong state")
	}

	return nil
}

func (c *Conn) LocalAddr() string {
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
