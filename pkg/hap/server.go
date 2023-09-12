package hap

import (
	"bufio"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/AlexxIT/go2rtc/pkg/hap/chacha20poly1305"
	"github.com/AlexxIT/go2rtc/pkg/hap/curve25519"
	"github.com/AlexxIT/go2rtc/pkg/hap/ed25519"
	"github.com/AlexxIT/go2rtc/pkg/hap/hkdf"
	"github.com/AlexxIT/go2rtc/pkg/hap/secure"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
)

type HandlerFunc func(net.Conn) error

type Server struct {
	Pin           string
	DeviceID      string
	DevicePrivate []byte

	GetPair func(conn net.Conn, id string) []byte
	AddPair func(conn net.Conn, id string, public []byte, permissions byte)

	Handler HandlerFunc
}

func (s *Server) ServerPublic() []byte {
	return s.DevicePrivate[32:]
}

//func (s *Server) Status() string {
//	if len(s.Pairings) == 0 {
//		return StatusNotPaired
//	}
//	return StatusPaired
//}

func (s *Server) SetupHash() string {
	// should be setup_id (random 4 alphanum) + device_id (mac address)
	// but device_id is random, so OK
	b := sha512.Sum512([]byte(s.DeviceID))
	return base64.StdEncoding.EncodeToString(b[:4])
}

func (s *Server) PairVerify(req *http.Request, rw *bufio.ReadWriter, conn net.Conn) error {
	// Request from iPhone
	var plainM1 struct {
		PublicKey string `tlv8:"3"`
		State     byte   `tlv8:"6"`
	}
	if err := tlv8.UnmarshalReader(io.LimitReader(rw, req.ContentLength), &plainM1); err != nil {
		return err
	}
	if plainM1.State != StateM1 {
		return newRequestError(plainM1)
	}

	// Generate the key pair
	sessionPublic, sessionPrivate := curve25519.GenerateKeyPair()
	sessionShared, err := curve25519.SharedSecret(sessionPrivate, []byte(plainM1.PublicKey))
	if err != nil {
		return err
	}

	encryptKey, err := hkdf.Sha512(
		sessionShared, "Pair-Verify-Encrypt-Salt", "Pair-Verify-Encrypt-Info",
	)
	if err != nil {
		return err
	}

	b := Append(sessionPublic, s.DeviceID, plainM1.PublicKey)
	signature, err := ed25519.Signature(s.DevicePrivate, b)
	if err != nil {
		return err
	}

	// STEP M2. Response to iPhone
	plainM2 := struct {
		Identifier string `tlv8:"1"`
		Signature  string `tlv8:"10"`
	}{
		Identifier: s.DeviceID,
		Signature:  string(signature),
	}
	if b, err = tlv8.Marshal(plainM2); err != nil {
		return err
	}

	b, err = chacha20poly1305.Encrypt(encryptKey, "PV-Msg02", b)
	if err != nil {
		return err
	}

	cipherM2 := struct {
		State         byte   `tlv8:"6"`
		PublicKey     string `tlv8:"3"`
		EncryptedData string `tlv8:"5"`
	}{
		State:         StateM2,
		PublicKey:     string(sessionPublic),
		EncryptedData: string(b),
	}
	body, err := tlv8.Marshal(cipherM2)
	if err != nil {
		return err
	}
	if err = WriteResponse(rw.Writer, http.StatusOK, MimeTLV8, body); err != nil {
		return err
	}

	// STEP M3. Request from iPhone
	if req, err = http.ReadRequest(rw.Reader); err != nil {
		return err
	}

	var cipherM3 struct {
		EncryptedData string `tlv8:"5"`
		State         byte   `tlv8:"6"`
	}
	if err = tlv8.UnmarshalReader(req.Body, &cipherM3); err != nil {
		return err
	}
	if cipherM3.State != StateM3 {
		return newRequestError(cipherM3)
	}

	if b, err = chacha20poly1305.Decrypt(encryptKey, "PV-Msg03", []byte(cipherM3.EncryptedData)); err != nil {
		return err
	}

	var plainM3 struct {
		Identifier string `tlv8:"1"`
		Signature  string `tlv8:"10"`
	}
	if err = tlv8.Unmarshal(b, &plainM3); err != nil {
		return err
	}

	clientPublic := s.GetPair(conn, plainM3.Identifier)
	if clientPublic == nil {
		return fmt.Errorf("hap: PairVerify from: %s, with unknown client_id: %s", conn.RemoteAddr(), plainM3.Identifier)
	}

	b = Append(plainM1.PublicKey, plainM3.Identifier, sessionPublic)
	if !ed25519.ValidateSignature(clientPublic, b, []byte(plainM3.Signature)) {
		return errors.New("new: ValidateSignature")
	}

	// STEP M4. Response to iPhone
	payloadM4 := struct {
		State byte `tlv8:"6"`
	}{
		State: StateM4,
	}
	if body, err = tlv8.Marshal(payloadM4); err != nil {
		return err
	}
	if err = WriteResponse(rw.Writer, http.StatusOK, MimeTLV8, body); err != nil {
		return err
	}

	if conn, err = secure.Client(conn, sessionShared, false); err != nil {
		return err
	}

	return s.Handler(conn)
}
