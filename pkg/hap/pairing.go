package hap

import (
	"bufio"
	"crypto/sha512"
	"errors"
	"github.com/brutella/hap"
	"github.com/brutella/hap/chacha20poly1305"
	"github.com/brutella/hap/curve25519"
	"github.com/brutella/hap/ed25519"
	"github.com/brutella/hap/hkdf"
	"github.com/brutella/hap/tlv8"
	"github.com/tadglines/go-pkgs/crypto/srp"
	"net"
	"net/http"
)

type pairSetupPayload struct {
	Method        byte   `tlv8:"0"`
	Identifier    string `tlv8:"1"`
	Salt          []byte `tlv8:"2"`
	PublicKey     []byte `tlv8:"3"`
	Proof         []byte `tlv8:"4"`
	EncryptedData []byte `tlv8:"5"`
	State         byte   `tlv8:"6"`
	Error         byte   `tlv8:"7"`
	RetryDelay    byte   `tlv8:"8"`
	Certificate   []byte `tlv8:"9"`
	Signature     []byte `tlv8:"10"`
	Permissions   byte   `tlv8:"11"`
	FragmentData  []byte `tlv8:"13"`
	FragmentLast  []byte `tlv8:"14"`
}

func (s *Server) PairSetupHandler(
	conn net.Conn, req *http.Request,
) (clientID string, err error) {
	// STEP 1. Request from iPhone
	payloadM1 := pairSetupPayload{}
	if err = tlv8.UnmarshalReader(req.Body, &payloadM1); err != nil {
		return
	}
	if payloadM1.State != hap.M1 {
		err = errors.New("wrong state")
		return
	}

	// generate our session public and salt using PIN
	username := []byte("Pair-Setup")

	var SRP *srp.SRP
	if SRP, err = srp.NewSRP(
		"rfc5054.3072", sha512.New,
		keyDerivativeFuncRFC2945(username),
	); err != nil {
		return
	}

	SRP.SaltLength = 16
	var salt, verifier []byte
	if salt, verifier, err = SRP.ComputeVerifier([]byte(s.Pin)); err != nil {
		return
	}
	session := SRP.NewServerSession(username, salt, verifier)

	// STEP 2. Response to iPhone
	payloadM2 := struct {
		Salt      []byte `tlv8:"2"`
		PublicKey []byte `tlv8:"3"`
		State     byte   `tlv8:"6"`
	}{
		State:     hap.M2,
		PublicKey: session.GetB(),
		Salt:      salt,
	}
	var buf []byte
	if buf, err = tlv8.Marshal(payloadM2); err != nil {
		return
	}
	if err = WriteResponse(conn, http.StatusOK, MimeTLV8, buf); err != nil {
		return
	}

	// STEP 3. Request from iPhone
	r := bufio.NewReader(conn)
	if req, err = http.ReadRequest(r); err != nil {
		return
	}
	payloadM3 := pairSetupPayload{}
	if err = tlv8.UnmarshalReader(req.Body, &payloadM3); err != nil {
		return
	}
	if payloadM3.State != hap.M3 {
		err = errors.New("wrong state")
		return
	}

	// important to compute key before verify client
	var sessionShared []byte
	if sessionShared, err = session.ComputeKey(payloadM3.PublicKey); err != nil {
		return
	}

	// support skip pin verify (any pin accepted)
	if s.Pin != "" && !session.VerifyClientAuthenticator(payloadM3.Proof) {
		err = errors.New("client proof is invalid")
		return
	}

	serverProof := session.ComputeAuthenticator(payloadM3.Proof)

	// STEP 4. Response to iPhone
	payloadM4 := struct {
		Proof []byte `tlv8:"4"`
		State byte   `tlv8:"6"`
	}{
		State: hap.M4, Proof: serverProof,
	}
	if buf, err = tlv8.Marshal(payloadM4); err != nil {
		return
	}
	if err = WriteResponse(conn, http.StatusOK, MimeTLV8, buf); err != nil {
		return
	}

	// STEP 5. Request from iPhone
	if req, err = http.ReadRequest(r); err != nil {
		return
	}
	encryptedM5 := pairSetupPayload{}
	if err = tlv8.UnmarshalReader(req.Body, &encryptedM5); err != nil {
		return
	}
	if encryptedM5.State != hap.M5 {
		err = errors.New("wrong state")
		return
	}

	msg := encryptedM5.EncryptedData[:len(encryptedM5.EncryptedData)-16]
	var mac [16]byte
	copy(mac[:], encryptedM5.EncryptedData[len(msg):]) // 16 byte (MAC)

	// decrypt message using session shared
	var sessionKey [32]byte
	if sessionKey, err = hkdf.Sha512(
		sessionShared, []byte("Pair-Setup-Encrypt-Salt"),
		[]byte("Pair-Setup-Encrypt-Info"),
	); err != nil {
		return
	}

	if buf, err = chacha20poly1305.DecryptAndVerify(
		sessionKey[:], []byte("PS-Msg05"), msg, mac, nil,
	); err != nil {
		return
	}

	// unpack message from TLV8
	payloadM5 := struct {
		Identifier string `tlv8:"1"`
		PublicKey  []byte `tlv8:"3"`
		Signature  []byte `tlv8:"10"`
	}{}
	if err = tlv8.Unmarshal(buf, &payloadM5); err != nil {
		return
	}

	// 3. verify client ID and Public
	var saltKey [32]byte
	if saltKey, err = hkdf.Sha512(
		sessionShared, []byte("Pair-Setup-Controller-Sign-Salt"),
		[]byte("Pair-Setup-Controller-Sign-Info"),
	); err != nil {
		return
	}

	buf = nil
	buf = append(buf, saltKey[:]...)
	buf = append(buf, payloadM5.Identifier...)
	buf = append(buf, payloadM5.PublicKey[:]...)

	if !ed25519.ValidateSignature(
		payloadM5.PublicKey[:], buf, payloadM5.Signature,
	) {
		err = errors.New("wrong client signature")
		return
	}

	// 4. generate signature to our ID adn Public
	if saltKey, err = hkdf.Sha512(
		sessionShared, []byte("Pair-Setup-Accessory-Sign-Salt"),
		[]byte("Pair-Setup-Accessory-Sign-Info"),
	); err != nil {
		return
	}

	buf = nil
	buf = append(buf, saltKey[:]...)
	buf = append(buf, []byte(s.ServerID)...)
	buf = append(buf, s.ServerPrivate[32:]...) // ServerPublic

	var signature []byte
	if signature, err = ed25519.Signature(s.ServerPrivate, buf); err != nil {
		return
	}

	// 5. pack our ID and Public
	payloadM6 := struct {
		Identifier []byte `tlv8:"1"`
		PublicKey  []byte `tlv8:"3"`
		Signature  []byte `tlv8:"10"`
	}{
		Identifier: []byte(s.ServerID),
		PublicKey:  s.ServerPrivate[32:],
		Signature:  signature,
	}
	if buf, err = tlv8.Marshal(payloadM6); err != nil {
		return
	}

	// 6. encrypt message
	buf, mac, _ = chacha20poly1305.EncryptAndSeal(
		sessionKey[:], []byte("PS-Msg06"), buf, nil,
	)

	// STEP 6. Response to iPhone
	encryptedM6 := struct {
		EncryptedData []byte `tlv8:"5"`
		State         byte   `tlv8:"6"`
	}{
		State:         hap.M6,
		EncryptedData: append(buf, mac[:]...),
	}
	if buf, err = tlv8.Marshal(encryptedM6); err != nil {
		return
	}
	if err = WriteResponse(conn, http.StatusOK, MimeTLV8, buf); err != nil {
		return
	}

	if s.Pairings != nil {
		s.Pairings[payloadM5.Identifier] = append(
			payloadM5.PublicKey, 1, // adds admin (1) flag
		)
	}

	clientID = payloadM5.Identifier

	return
}

func keyDerivativeFuncRFC2945(username []byte) srp.KeyDerivationFunc {
	return func(salt, pin []byte) []byte {
		h := sha512.New()
		h.Write(username)
		h.Write([]byte(":"))
		h.Write(pin)
		t2 := h.Sum(nil)
		h.Reset()
		h.Write(salt)
		h.Write(t2)
		return h.Sum(nil)
	}
}

type pairVerifyPayload struct {
	Method        byte   `tlv8:"0"`
	Identifier    string `tlv8:"1"`
	PublicKey     []byte `tlv8:"3"`
	EncryptedData []byte `tlv8:"5"`
	State         byte   `tlv8:"6"`
	Signature     []byte `tlv8:"10"`
}

func (s *Server) PairVerifyHandler(
	conn net.Conn, req *http.Request,
) (secure *Secure, err error) {
	// STEP M1. Request from iPhone
	payloadM1 := pairVerifyPayload{}
	if err = tlv8.UnmarshalReader(req.Body, &payloadM1); err != nil {
		return
	}
	if payloadM1.State != hap.M1 {
		err = errors.New("wrong state")
		return
	}

	var clientPublic [32]byte
	copy(clientPublic[:], payloadM1.PublicKey)

	// Generate the key pair.
	sessionPublic, sessionPrivate := curve25519.GenerateKeyPair()
	sessionShared := curve25519.SharedSecret(sessionPrivate, clientPublic)

	var sessionKey [32]byte
	if sessionKey, err = hkdf.Sha512(
		sessionShared[:], []byte("Pair-Verify-Encrypt-Salt"),
		[]byte("Pair-Verify-Encrypt-Info"),
	); err != nil {
		return
	}

	var buf []byte
	buf = append(buf, sessionPublic[:]...)
	buf = append(buf, s.ServerID...)
	buf = append(buf, clientPublic[:]...)

	var signature []byte
	if signature, err = ed25519.Signature(s.ServerPrivate[:], buf); err != nil {
		return
	}

	// STEP M2. Response to iPhone
	payloadM2 := struct {
		Identifier string `tlv8:"1"`
		Signature  []byte `tlv8:"10"`
	}{
		Identifier: s.ServerID,
		Signature:  signature,
	}
	if buf, err = tlv8.Marshal(payloadM2); err != nil {
		return
	}

	var mac [16]byte
	buf, mac, _ = chacha20poly1305.EncryptAndSeal(
		sessionKey[:], []byte("PV-Msg02"), buf, nil,
	)
	encryptedM2 := struct {
		State         byte   `tlv8:"6"`
		PublicKey     []byte `tlv8:"3"`
		EncryptedData []byte `tlv8:"5"`
	}{
		State:         hap.M2,
		PublicKey:     sessionPublic[:],
		EncryptedData: append(buf, mac[:]...),
	}
	if buf, err = tlv8.Marshal(encryptedM2); err != nil {
		return
	}
	if err = WriteResponse(conn, http.StatusOK, MimeTLV8, buf); err != nil {
		return
	}

	// STEP M3. Request from iPhone
	r := bufio.NewReader(conn)
	if req, err = http.ReadRequest(r); err != nil {
		return
	}
	encryptedM3 := pairSetupPayload{}
	if err = tlv8.UnmarshalReader(req.Body, &encryptedM3); err != nil {
		return
	}
	if encryptedM3.State != hap.M3 {
		err = errors.New("wrong state")
		return
	}

	buf = encryptedM3.EncryptedData[:len(encryptedM3.EncryptedData)-16]
	copy(mac[:], encryptedM3.EncryptedData[len(buf):]) // 16 byte (MAC)

	if buf, err = chacha20poly1305.DecryptAndVerify(
		sessionKey[:], []byte("PV-Msg03"), buf, mac, nil,
	); err != nil {
		return
	}

	payloadM3 := pairVerifyPayload{}
	if err = tlv8.Unmarshal(buf, &payloadM3); err != nil {
		return
	}

	if s.Pairings != nil {
		pairing := s.Pairings[payloadM3.Identifier]
		if pairing == nil {
			err = errors.New("not paired yet")
			return
		}

		buf = nil
		buf = append(buf, clientPublic[:]...)
		buf = append(buf, []byte(payloadM3.Identifier)...)
		buf = append(buf, sessionPublic[:]...)

		if !ed25519.ValidateSignature(
			pairing[:32], buf, payloadM3.Signature,
		) {
			err = errors.New("signature invalid")
			return
		}
	}

	// STEP M4. Response to iPhone
	payloadM4 := struct {
		State byte `tlv8:"6"`
	}{
		State: hap.M4,
	}
	if buf, err = tlv8.Marshal(payloadM4); err != nil {
		return
	}
	err = WriteResponse(conn, http.StatusOK, MimeTLV8, buf)

	if secure, err = NewSecure(sessionShared, true); err != nil {
		return
	}
	secure.Conn = conn

	return
}
