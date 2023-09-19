package hap

import (
	"bufio"
	"crypto/sha512"
	"errors"
	"net"
	"net/url"

	"github.com/AlexxIT/go2rtc/pkg/hap/chacha20poly1305"
	"github.com/AlexxIT/go2rtc/pkg/hap/ed25519"
	"github.com/AlexxIT/go2rtc/pkg/hap/hkdf"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
	"github.com/tadglines/go-pkgs/crypto/srp"
)

// Pair homekit
func Pair(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()

	c := &Client{
		DeviceAddress: u.Host,
		DeviceID:      query.Get("device_id"),
		ClientID:      query.Get("client_id"),
		ClientPrivate: DecodeKey(query.Get("client_private")),
	}

	if c.ClientID == "" {
		c.ClientID = GenerateUUID()
	}
	if c.ClientPrivate == nil {
		c.ClientPrivate = GenerateKey()
	}

	if err = c.Pair(query.Get("feature"), query.Get("pin")); err != nil {
		return nil, err
	}

	return c, nil
}

func Unpair(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	query := u.Query()
	conn := &Client{
		DeviceAddress: u.Host,
		DeviceID:      query.Get("device_id"),
		DevicePublic:  DecodeKey(query.Get("device_public")),
		ClientID:      query.Get("client_id"),
		ClientPrivate: DecodeKey(query.Get("client_private")),
	}

	if err = conn.Dial(); err != nil {
		return err
	}

	defer conn.Close()

	if err = conn.ListPairings(); err != nil {
		return err
	}

	return conn.DeletePairing(conn.ClientID)
}

func (c *Client) Pair(feature, pin string) (err error) {
	if pin, err = SanitizePin(pin); err != nil {
		return err
	}

	c.Conn, err = net.DialTimeout("tcp", c.DeviceAddress, ConnDialTimeout)
	if err != nil {
		return
	}

	c.reader = bufio.NewReader(c.Conn)

	// STEP M1. Send HELLO
	plainM1 := struct {
		Method byte `tlv8:"0"`
		State  byte `tlv8:"6"`
	}{
		Method: MethodPair,
		State:  StateM1,
	}
	if feature == "1" {
		plainM1.Method = MethodPairMFi // ff=1 => method=1, ff=2 => method=0
	}
	res, err := c.Post(PathPairSetup, MimeTLV8, tlv8.MarshalReader(plainM1))
	if err != nil {
		return
	}

	// STEP M2. Read Device Salt and session PublicKey
	var plainM2 struct {
		Salt       string `tlv8:"2"`
		SessionKey string `tlv8:"3"` // server public key, aka session.B
		State      byte   `tlv8:"6"`
		Error      byte   `tlv8:"7"`
	}
	if err = tlv8.UnmarshalReader(res.Body, &plainM2); err != nil {
		return
	}
	if plainM2.State != StateM2 {
		return newResponseError(plainM1, plainM2)
	}
	if plainM2.Error != 0 {
		return newPairingError(plainM2.Error)
	}

	// STEP M3. Generate SRP Session using pin
	username := []byte("Pair-Setup")

	// Stanford Secure Remote Password (SRP) / Password Authenticated Key Exchange (PAKE)
	pake, err := srp.NewSRP(
		"rfc5054.3072", sha512.New, keyDerivativeFuncRFC2945(username),
	)
	if err != nil {
		return
	}

	pake.SaltLength = 16

	// username: "Pair-Setup", password: PIN (with dashes)
	session := pake.NewClientSession(username, []byte(pin))
	sessionShared, err := session.ComputeKey([]byte(plainM2.Salt), []byte(plainM2.SessionKey))
	if err != nil {
		return
	}

	// STEP M3. Send request
	plainM3 := struct {
		SessionKey string `tlv8:"3"`
		Proof      string `tlv8:"4"`
		State      byte   `tlv8:"6"`
	}{
		SessionKey: string(session.GetA()), // client public key, aka session.A
		Proof:      string(session.ComputeAuthenticator()),
		State:      StateM3,
	}
	if res, err = c.Post(PathPairSetup, MimeTLV8, tlv8.MarshalReader(plainM3)); err != nil {
		return
	}

	// STEP M4. Read response
	var plainM4 struct {
		Proof string `tlv8:"4"` // server proof
		State byte   `tlv8:"6"`
		Error byte   `tlv8:"7"`

		EncryptedData string `tlv8:"5"` // skip EncryptedData validation (for MFi devices)
	}
	if err = tlv8.UnmarshalReader(res.Body, &plainM4); err != nil {
		return
	}
	if plainM4.State != StateM4 {
		return newResponseError(plainM3, plainM4)
	}
	if plainM4.Error != 0 {
		return newPairingError(plainM4.Error)
	}

	// STEP M4. Verify response
	if !session.VerifyServerAuthenticator([]byte(plainM4.Proof)) {
		return errors.New("hap: VerifyServerAuthenticator")
	}

	// STEP M5. Generate signature
	localSign, err := hkdf.Sha512(
		sessionShared, "Pair-Setup-Controller-Sign-Salt", "Pair-Setup-Controller-Sign-Info",
	)
	if err != nil {
		return
	}

	b := Append(localSign, c.ClientID, c.ClientPublic())
	signature, err := ed25519.Signature(c.ClientPrivate, b)
	if err != nil {
		return
	}

	// STEP M5. Generate payload
	plainM5 := struct {
		Identifier string `tlv8:"1"`
		PublicKey  string `tlv8:"3"`
		Signature  string `tlv8:"10"`
	}{
		Identifier: c.ClientID,
		PublicKey:  string(c.ClientPublic()),
		Signature:  string(signature),
	}
	if b, err = tlv8.Marshal(plainM5); err != nil {
		return
	}

	// STEP M5. Encrypt payload
	encryptKey, err := hkdf.Sha512(
		sessionShared, "Pair-Setup-Encrypt-Salt", "Pair-Setup-Encrypt-Info",
	)
	if err != nil {
		return
	}

	if b, err = chacha20poly1305.Encrypt(encryptKey, "PS-Msg05", b); err != nil {
		return
	}

	// STEP M5. Send request
	cipherM5 := struct {
		EncryptedData string `tlv8:"5"`
		State         byte   `tlv8:"6"`
	}{
		EncryptedData: string(b),
		State:         StateM5,
	}
	if res, err = c.Post(PathPairSetup, MimeTLV8, tlv8.MarshalReader(cipherM5)); err != nil {
		return
	}

	// STEP M6. Read response
	cipherM6 := struct {
		EncryptedData string `tlv8:"5"`
		State         byte   `tlv8:"6"`
		Error         byte   `tlv8:"7"`
	}{}
	if err = tlv8.UnmarshalReader(res.Body, &cipherM6); err != nil {
		return
	}
	if cipherM6.State != StateM6 || cipherM6.Error != 0 {
		return newResponseError(plainM5, cipherM6)
	}

	// STEP M6. Decrypt payload
	b, err = chacha20poly1305.Decrypt(encryptKey, "PS-Msg06", []byte(cipherM6.EncryptedData))
	if err != nil {
		return
	}

	plainM6 := struct {
		Identifier string `tlv8:"1"`
		PublicKey  string `tlv8:"3"`
		Signature  string `tlv8:"10"`
	}{}
	if err = tlv8.Unmarshal(b, &plainM6); err != nil {
		return
	}

	// STEP M6. Verify payload
	remoteSign, err := hkdf.Sha512(
		sessionShared, "Pair-Setup-Accessory-Sign-Salt", "Pair-Setup-Accessory-Sign-Info",
	)
	if err != nil {
		return
	}

	b = Append(remoteSign, plainM6.Identifier, plainM6.PublicKey)
	if !ed25519.ValidateSignature([]byte(plainM6.PublicKey), b, []byte(plainM6.Signature)) {
		return errors.New("hap: ValidateSignature")
	}

	if c.DeviceID != plainM6.Identifier {
		return errors.New("hap: wrong DeviceID: " + plainM6.Identifier)
	}

	c.DevicePublic = []byte(plainM6.PublicKey)

	return nil
}

func (c *Client) ListPairings() error {
	plainM1 := struct {
		Method byte `tlv8:"0"`
		State  byte `tlv8:"6"`
	}{
		Method: MethodListPairings,
		State:  StateM1,
	}
	res, err := c.Post(PathPairings, MimeTLV8, tlv8.MarshalReader(plainM1))
	if err != nil {
		return err
	}

	// TODO: don't know how to fix array of items
	var plainM2 struct {
		Identifier string `tlv8:"1"`
		PublicKey  string `tlv8:"3"`
		State      byte   `tlv8:"6"`
		Permission byte   `tlv8:"11"`
	}
	if err = tlv8.UnmarshalReader(res.Body, &plainM2); err != nil {
		return err
	}

	return nil
}

func (c *Client) PairingsAdd(clientID string, clientPublic []byte, admin bool) error {
	plainM1 := struct {
		Method     byte   `tlv8:"0"`
		Identifier string `tlv8:"1"`
		PublicKey  string `tlv8:"3"`
		State      byte   `tlv8:"6"`
		Permission byte   `tlv8:"11"`
	}{
		Method:     MethodAddPairing,
		Identifier: clientID,
		PublicKey:  string(clientPublic),
		State:      StateM1,
		Permission: PermissionUser,
	}
	if admin {
		plainM1.Permission = PermissionAdmin
	}
	res, err := c.Post(PathPairings, MimeTLV8, tlv8.MarshalReader(plainM1))
	if err != nil {
		return err
	}

	var plainM2 struct {
		State   byte `tlv8:"6"`
		Unknown byte `tlv8:"7"`
	}
	if err = tlv8.UnmarshalReader(res.Body, &plainM2); err != nil {
		return err
	}

	return nil
}

func (c *Client) DeletePairing(id string) error {
	plainM1 := struct {
		Method     byte   `tlv8:"0"`
		Identifier string `tlv8:"1"`
		State      byte   `tlv8:"6"`
	}{
		Method:     MethodDeletePairing,
		Identifier: id,
		State:      StateM1,
	}
	res, err := c.Post(PathPairings, MimeTLV8, tlv8.MarshalReader(plainM1))
	if err != nil {
		return err
	}

	var plainM2 struct {
		State byte `tlv8:"6"`
	}
	if err = tlv8.UnmarshalReader(res.Body, &plainM2); err != nil {
		return err
	}
	if plainM2.State != StateM2 {
		return newResponseError(plainM1, plainM2)
	}

	return nil
}

func newPairingError(code byte) error {
	var text string
	// https://github.com/apple/HomeKitADK/blob/fb201f98f5fdc7fef6a455054f08b59cca5d1ec8/HAP/HAPPairing.h#L89
	switch code {
	case 1:
		text = "Generic error to handle unexpected errors"
	case 2:
		text = "Setup code or signature verification failed"
	case 3:
		text = "Client must look at the retry delay TLV item and wait that many seconds before retrying"
	case 4:
		text = "Server cannot accept any more pairings"
	case 5:
		text = "Server reached its maximum number of authentication attempts"
	case 6:
		text = "Server pairing method is unavailable"
	case 7:
		text = "Server is busy and cannot accept a pairing request at this time"
	default:
		text = "Unknown pairing error"
	}
	return errors.New("hap: " + text)
}

func keyDerivativeFuncRFC2945(username []byte) srp.KeyDerivationFunc {
	return func(salt, password []byte) []byte {
		h1 := sha512.New()
		h1.Write(username)
		h1.Write([]byte(":"))
		h1.Write(password)

		h2 := sha512.New()
		h2.Write(salt)
		h2.Write(h1.Sum(nil))

		return h2.Sum(nil)
	}
}
