package hap

import (
	"bufio"
	"crypto/sha512"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/AlexxIT/go2rtc/pkg/hap/chacha20poly1305"
	"github.com/AlexxIT/go2rtc/pkg/hap/ed25519"
	"github.com/AlexxIT/go2rtc/pkg/hap/hkdf"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
	"github.com/tadglines/go-pkgs/crypto/srp"
)

const (
	PairMethodSetup = iota
	PairMethodSetupWithAuth
	PairMethodVerify
	PairMethodAdd
	PairMethodRemove
	PairMethodList
)

func (s *Server) PairSetup(req *http.Request, rw *bufio.ReadWriter, conn net.Conn) error {
	if req.Header.Get("Content-Type") != MimeTLV8 {
		return errors.New("hap: wrong content type")
	}

	// STEP 1. Request from iPhone
	var plainM1 struct {
		Method byte   `tlv8:"0"`
		State  byte   `tlv8:"6"`
		Flags  uint32 `tlv8:"19"`
	}
	if err := tlv8.UnmarshalReader(io.LimitReader(rw, req.ContentLength), &plainM1); err != nil {
		return err
	}
	if plainM1.State != StateM1 {
		return newRequestError(plainM1)
	}

	username := []byte("Pair-Setup")

	// Stanford Secure Remote Password (SRP) / Password Authenticated Key Exchange (PAKE)
	pake, err := srp.NewSRP(
		"rfc5054.3072", sha512.New, keyDerivativeFuncRFC2945(username),
	)
	if err != nil {
		return err
	}

	pake.SaltLength = 16

	salt, verifier, err := pake.ComputeVerifier([]byte(s.Pin))

	session := pake.NewServerSession(username, salt, verifier)

	// STEP 2. Response to iPhone
	plainM2 := struct {
		Salt      string `tlv8:"2"`
		PublicKey string `tlv8:"3"`
		State     byte   `tlv8:"6"`
	}{
		State:     StateM2,
		PublicKey: string(session.GetB()),
		Salt:      string(salt),
	}
	body, err := tlv8.Marshal(plainM2)
	if err != nil {
		return err
	}
	if err = WriteResponse(rw.Writer, http.StatusOK, MimeTLV8, body); err != nil {
		return err
	}

	// STEP 3. Request from iPhone
	if req, err = http.ReadRequest(rw.Reader); err != nil {
		return err
	}

	var plainM3 struct {
		SessionKey string `tlv8:"3"`
		Proof      string `tlv8:"4"`
		State      byte   `tlv8:"6"`
	}
	if err = tlv8.UnmarshalReader(req.Body, &plainM3); err != nil {
		return err
	}
	if plainM3.State != StateM3 {
		return newRequestError(plainM3)
	}

	// important to compute key before verify client
	sessionShared, err := session.ComputeKey([]byte(plainM3.SessionKey))
	if err != nil {
		return err
	}

	if !session.VerifyClientAuthenticator([]byte(plainM3.Proof)) {
		return errors.New("hap: VerifyClientAuthenticator")
	}

	proof := session.ComputeAuthenticator([]byte(plainM3.Proof)) // server proof

	// STEP 4. Response to iPhone
	payloadM4 := struct {
		Proof string `tlv8:"4"`
		State byte   `tlv8:"6"`
	}{
		Proof: string(proof),
		State: StateM4,
	}
	if body, err = tlv8.Marshal(payloadM4); err != nil {
		return err
	}
	if err = WriteResponse(rw.Writer, http.StatusOK, MimeTLV8, body); err != nil {
		return err
	}

	// STEP 5. Request from iPhone
	if req, err = http.ReadRequest(rw.Reader); err != nil {
		return err
	}
	var cipherM5 struct {
		EncryptedData string `tlv8:"5"`
		State         byte   `tlv8:"6"`
	}
	if err = tlv8.UnmarshalReader(req.Body, &cipherM5); err != nil {
		return err
	}
	if cipherM5.State != StateM5 {
		return newRequestError(cipherM5)
	}

	// decrypt message using session shared
	encryptKey, err := hkdf.Sha512(sessionShared, "Pair-Setup-Encrypt-Salt", "Pair-Setup-Encrypt-Info")
	if err != nil {
		return err
	}

	b, err := chacha20poly1305.Decrypt(encryptKey, "PS-Msg05", []byte(cipherM5.EncryptedData))
	if err != nil {
		return err
	}

	// unpack message from TLV8
	var plainM5 struct {
		Identifier string `tlv8:"1"`
		PublicKey  string `tlv8:"3"`
		Signature  string `tlv8:"10"`
	}
	if err = tlv8.Unmarshal(b, &plainM5); err != nil {
		return err
	}

	// 3. verify client ID and Public
	remoteSign, err := hkdf.Sha512(
		sessionShared, "Pair-Setup-Controller-Sign-Salt", "Pair-Setup-Controller-Sign-Info",
	)
	if err != nil {
		return err
	}

	b = Append(remoteSign, plainM5.Identifier, plainM5.PublicKey)
	if !ed25519.ValidateSignature([]byte(plainM5.PublicKey), b, []byte(plainM5.Signature)) {
		return errors.New("hap: ValidateSignature")
	}

	// 4. generate signature to our ID and Public
	localSign, err := hkdf.Sha512(
		sessionShared, "Pair-Setup-Accessory-Sign-Salt", "Pair-Setup-Accessory-Sign-Info",
	)
	if err != nil {
		return err
	}

	b = Append(localSign, s.DeviceID, s.ServerPublic()) // ServerPublic
	signature, err := ed25519.Signature(s.DevicePrivate, b)
	if err != nil {
		return err
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
		return err
	}

	// 6. encrypt message
	b, err = chacha20poly1305.Encrypt(encryptKey, "PS-Msg06", b)
	if err != nil {
		return err
	}

	// STEP 6. Response to iPhone
	cipherM6 := struct {
		EncryptedData string `tlv8:"5"`
		State         byte   `tlv8:"6"`
	}{
		State:         StateM6,
		EncryptedData: string(b),
	}
	if body, err = tlv8.Marshal(cipherM6); err != nil {
		return err
	}
	if err = WriteResponse(rw.Writer, http.StatusOK, MimeTLV8, body); err != nil {
		return err
	}

	s.AddPair(conn, plainM5.Identifier, []byte(plainM5.PublicKey), PermissionAdmin)

	return nil
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
