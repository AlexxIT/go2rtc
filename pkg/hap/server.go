package hap

import (
	"bufio"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	"github.com/AlexxIT/go2rtc/pkg/hap/chacha20poly1305"
	"github.com/AlexxIT/go2rtc/pkg/hap/curve25519"
	"github.com/AlexxIT/go2rtc/pkg/hap/ed25519"
	"github.com/AlexxIT/go2rtc/pkg/hap/hkdf"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
	"github.com/tadglines/go-pkgs/crypto/srp"
)

type Server struct {
	Pin           string
	DeviceID      string
	DevicePrivate []byte

	// GetClientPublic may be nil, so client validation will be disabled
	GetClientPublic func(id string) []byte
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

func (s *Server) PairSetup(req *http.Request, rw *bufio.ReadWriter) (id string, publicKey []byte, err error) {
	// STEP 1. Request from iPhone
	var plainM1 struct {
		State  byte   `tlv8:"6"`
		Method byte   `tlv8:"0"`
		Flags  uint32 `tlv8:"19"`
	}
	if err = tlv8.UnmarshalReader(req.Body, req.ContentLength, &plainM1); err != nil {
		return
	}
	if plainM1.State != StateM1 {
		err = newRequestError(plainM1)
		return
	}

	username := []byte("Pair-Setup")

	// Stanford Secure Remote Password (SRP) / Password Authenticated Key Exchange (PAKE)
	pake, err := srp.NewSRP("rfc5054.3072", sha512.New, keyDerivativeFuncRFC2945(username))
	if err != nil {
		return
	}

	pake.SaltLength = 16

	salt, verifier, err := pake.ComputeVerifier([]byte(s.Pin))
	if err != nil {
		return
	}

	session := pake.NewServerSession(username, salt, verifier)

	// STEP 2. Response to iPhone
	plainM2 := struct {
		State     byte   `tlv8:"6"`
		PublicKey string `tlv8:"3"`
		Salt      string `tlv8:"2"`
	}{
		State:     StateM2,
		PublicKey: string(session.GetB()),
		Salt:      string(salt),
	}
	body, err := tlv8.Marshal(plainM2)
	if err != nil {
		return
	}
	if err = WriteResponse(rw.Writer, http.StatusOK, MimeTLV8, body); err != nil {
		return
	}

	// STEP 3. Request from iPhone
	if req, err = http.ReadRequest(rw.Reader); err != nil {
		return
	}

	var plainM3 struct {
		State     byte   `tlv8:"6"`
		PublicKey string `tlv8:"3"`
		Proof     string `tlv8:"4"`
	}
	if err = tlv8.UnmarshalReader(req.Body, req.ContentLength, &plainM3); err != nil {
		return
	}
	if plainM3.State != StateM3 {
		err = newRequestError(plainM3)
		return
	}

	// important to compute key before verify client
	sessionShared, err := session.ComputeKey([]byte(plainM3.PublicKey))
	if err != nil {
		return
	}

	if !session.VerifyClientAuthenticator([]byte(plainM3.Proof)) {
		err = errors.New("hap: VerifyClientAuthenticator")
		return
	}

	proof := session.ComputeAuthenticator([]byte(plainM3.Proof)) // server proof

	// STEP 4. Response to iPhone
	payloadM4 := struct {
		State byte   `tlv8:"6"`
		Proof string `tlv8:"4"`
	}{
		State: StateM4,
		Proof: string(proof),
	}
	if body, err = tlv8.Marshal(payloadM4); err != nil {
		return
	}
	if err = WriteResponse(rw.Writer, http.StatusOK, MimeTLV8, body); err != nil {
		return
	}

	// STEP 5. Request from iPhone
	if req, err = http.ReadRequest(rw.Reader); err != nil {
		return
	}
	var cipherM5 struct {
		State         byte   `tlv8:"6"`
		EncryptedData string `tlv8:"5"`
	}
	if err = tlv8.UnmarshalReader(req.Body, req.ContentLength, &cipherM5); err != nil {
		return
	}
	if cipherM5.State != StateM5 {
		err = newRequestError(cipherM5)
		return
	}

	// decrypt message using session shared
	encryptKey, err := hkdf.Sha512(sessionShared, "Pair-Setup-Encrypt-Salt", "Pair-Setup-Encrypt-Info")
	if err != nil {
		return
	}

	b, err := chacha20poly1305.Decrypt(encryptKey, "PS-Msg05", []byte(cipherM5.EncryptedData))
	if err != nil {
		return
	}

	// unpack message from TLV8
	var plainM5 struct {
		Identifier string `tlv8:"1"`
		PublicKey  string `tlv8:"3"`
		Signature  string `tlv8:"10"`
	}
	if err = tlv8.Unmarshal(b, &plainM5); err != nil {
		return
	}

	// 3. verify client ID and Public
	remoteSign, err := hkdf.Sha512(
		sessionShared, "Pair-Setup-Controller-Sign-Salt", "Pair-Setup-Controller-Sign-Info",
	)
	if err != nil {
		return
	}

	b = Append(remoteSign, plainM5.Identifier, plainM5.PublicKey)
	if !ed25519.ValidateSignature([]byte(plainM5.PublicKey), b, []byte(plainM5.Signature)) {
		err = errors.New("hap: ValidateSignature")
		return
	}

	// 4. generate signature to our ID and Public
	localSign, err := hkdf.Sha512(
		sessionShared, "Pair-Setup-Accessory-Sign-Salt", "Pair-Setup-Accessory-Sign-Info",
	)
	if err != nil {
		return
	}

	b = Append(localSign, s.DeviceID, s.ServerPublic()) // ServerPublic
	signature, err := ed25519.Signature(s.DevicePrivate, b)
	if err != nil {
		return
	}

	// 5. pack our ID and Public
	plainM6 := struct {
		Identifier string `tlv8:"1"`
		PublicKey  string `tlv8:"3"`
		Signature  string `tlv8:"10"`
	}{
		Identifier: s.DeviceID,
		PublicKey:  string(s.ServerPublic()),
		Signature:  string(signature),
	}
	if b, err = tlv8.Marshal(plainM6); err != nil {
		return
	}

	// 6. encrypt message
	b, err = chacha20poly1305.Encrypt(encryptKey, "PS-Msg06", b)
	if err != nil {
		return
	}

	// STEP 6. Response to iPhone
	cipherM6 := struct {
		State         byte   `tlv8:"6"`
		EncryptedData string `tlv8:"5"`
	}{
		State:         StateM6,
		EncryptedData: string(b),
	}
	if body, err = tlv8.Marshal(cipherM6); err != nil {
		return
	}
	if err = WriteResponse(rw.Writer, http.StatusOK, MimeTLV8, body); err != nil {
		return
	}

	id = plainM5.Identifier
	publicKey = []byte(plainM5.PublicKey)

	return
}

func (s *Server) PairVerify(req *http.Request, rw *bufio.ReadWriter) (id string, sessionKey []byte, err error) {
	// Request from iPhone
	var plainM1 struct {
		State     byte   `tlv8:"6"`
		PublicKey string `tlv8:"3"`
	}
	if err = tlv8.UnmarshalReader(req.Body, req.ContentLength, &plainM1); err != nil {
		return
	}
	if plainM1.State != StateM1 {
		err = newRequestError(plainM1)
		return
	}

	// Generate the key pair
	sessionPublic, sessionPrivate := curve25519.GenerateKeyPair()
	sessionShared, err := curve25519.SharedSecret(sessionPrivate, []byte(plainM1.PublicKey))
	if err != nil {
		return
	}

	encryptKey, err := hkdf.Sha512(
		sessionShared, "Pair-Verify-Encrypt-Salt", "Pair-Verify-Encrypt-Info",
	)
	if err != nil {
		return
	}

	b := Append(sessionPublic, s.DeviceID, plainM1.PublicKey)
	signature, err := ed25519.Signature(s.DevicePrivate, b)
	if err != nil {
		return
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
		return
	}

	b, err = chacha20poly1305.Encrypt(encryptKey, "PV-Msg02", b)
	if err != nil {
		return
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
		return
	}
	if err = WriteResponse(rw.Writer, http.StatusOK, MimeTLV8, body); err != nil {
		return
	}

	// STEP M3. Request from iPhone
	if req, err = http.ReadRequest(rw.Reader); err != nil {
		return
	}

	var cipherM3 struct {
		State         byte   `tlv8:"6"`
		EncryptedData string `tlv8:"5"`
	}
	if err = tlv8.UnmarshalReader(req.Body, req.ContentLength, &cipherM3); err != nil {
		return
	}
	if cipherM3.State != StateM3 {
		err = newRequestError(cipherM3)
		return
	}

	b, err = chacha20poly1305.Decrypt(encryptKey, "PV-Msg03", []byte(cipherM3.EncryptedData))
	if err != nil {
		return
	}

	var plainM3 struct {
		Identifier string `tlv8:"1"`
		Signature  string `tlv8:"10"`
	}
	if err = tlv8.Unmarshal(b, &plainM3); err != nil {
		return
	}

	if s.GetClientPublic != nil {
		clientPublic := s.GetClientPublic(plainM3.Identifier)
		if clientPublic == nil {
			err = errors.New("hap: PairVerify with unknown client_id: " + plainM3.Identifier)
			return
		}

		b = Append(plainM1.PublicKey, plainM3.Identifier, sessionPublic)
		if !ed25519.ValidateSignature(clientPublic, b, []byte(plainM3.Signature)) {
			err = errors.New("hap: ValidateSignature")
			return
		}
	}

	// STEP M4. Response to iPhone
	payloadM4 := struct {
		State byte `tlv8:"6"`
	}{
		State: StateM4,
	}
	if body, err = tlv8.Marshal(payloadM4); err != nil {
		return
	}
	if err = WriteResponse(rw.Writer, http.StatusOK, MimeTLV8, body); err != nil {
		return
	}

	id = plainM3.Identifier
	sessionKey = sessionShared

	return
}

func WriteResponse(w *bufio.Writer, statusCode int, contentType string, body []byte) error {
	header := fmt.Sprintf(
		"HTTP/1.1 %d %s\r\nContent-Type: %s\r\nContent-Length: %d\r\n\r\n",
		statusCode, http.StatusText(statusCode), contentType, len(body),
	)
	body = append([]byte(header), body...)
	if _, err := w.Write(body); err != nil {
		return err
	}
	return w.Flush()
}

//func WriteBackoff(rw *bufio.ReadWriter) error {
//	plainM2 := struct {
//		State byte `tlv8:"6"`
//		Error byte `tlv8:"7"`
//	}{
//		State: StateM2,
//		Error: 3, // BackoffError
//	}
//	body, err := tlv8.Marshal(plainM2)
//	if err != nil {
//		return err
//	}
//	return WriteResponse(rw.Writer, http.StatusOK, MimeTLV8, body)
//}
