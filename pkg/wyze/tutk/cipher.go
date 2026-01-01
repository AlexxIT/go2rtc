package tutk

import (
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"sync/atomic"

	"github.com/pion/dtls/v3"
	"github.com/pion/dtls/v3/pkg/crypto/clientcertificate"
	"github.com/pion/dtls/v3/pkg/crypto/prf"
	"github.com/pion/dtls/v3/pkg/protocol"
	"github.com/pion/dtls/v3/pkg/protocol/recordlayer"
	"golang.org/x/crypto/chacha20poly1305"
)

const CipherSuiteID_CCAC dtls.CipherSuiteID = 0xCCAC

const (
	chachaTagLength   = 16
	chachaNonceLength = 12
)

var (
	errDecryptPacket      = &protocol.TemporaryError{Err: errors.New("failed to decrypt packet")}
	errCipherSuiteNotInit = &protocol.TemporaryError{Err: errors.New("CipherSuite not initialized")}
)

type ChaCha20Poly1305Cipher struct {
	localCipher, remoteCipher   cipher.AEAD
	localWriteIV, remoteWriteIV []byte
}

func NewChaCha20Poly1305Cipher(localKey, localWriteIV, remoteKey, remoteWriteIV []byte) (*ChaCha20Poly1305Cipher, error) {
	localCipher, err := chacha20poly1305.New(localKey)
	if err != nil {
		return nil, err
	}

	remoteCipher, err := chacha20poly1305.New(remoteKey)
	if err != nil {
		return nil, err
	}

	return &ChaCha20Poly1305Cipher{
		localCipher:   localCipher,
		localWriteIV:  localWriteIV,
		remoteCipher:  remoteCipher,
		remoteWriteIV: remoteWriteIV,
	}, nil
}

func generateAEADAdditionalData(h *recordlayer.Header, payloadLen int) []byte {
	var additionalData [13]byte

	binary.BigEndian.PutUint64(additionalData[:], h.SequenceNumber)
	binary.BigEndian.PutUint16(additionalData[:], h.Epoch)
	additionalData[8] = byte(h.ContentType)
	additionalData[9] = h.Version.Major
	additionalData[10] = h.Version.Minor
	binary.BigEndian.PutUint16(additionalData[11:], uint16(payloadLen))

	return additionalData[:]
}

func computeNonce(iv []byte, epoch uint16, sequenceNumber uint64) []byte {
	nonce := make([]byte, chachaNonceLength)

	binary.BigEndian.PutUint64(nonce[4:], sequenceNumber)
	binary.BigEndian.PutUint16(nonce[4:], epoch)

	for i := 0; i < chachaNonceLength; i++ {
		nonce[i] ^= iv[i]
	}

	return nonce
}

func (c *ChaCha20Poly1305Cipher) Encrypt(pkt *recordlayer.RecordLayer, raw []byte) ([]byte, error) {
	payload := raw[pkt.Header.Size():]
	raw = raw[:pkt.Header.Size()]

	nonce := computeNonce(c.localWriteIV, pkt.Header.Epoch, pkt.Header.SequenceNumber)
	additionalData := generateAEADAdditionalData(&pkt.Header, len(payload))
	encryptedPayload := c.localCipher.Seal(nil, nonce, payload, additionalData)

	r := make([]byte, len(raw)+len(encryptedPayload))
	copy(r, raw)
	copy(r[len(raw):], encryptedPayload)

	binary.BigEndian.PutUint16(r[pkt.Header.Size()-2:], uint16(len(r)-pkt.Header.Size()))

	return r, nil
}

func (c *ChaCha20Poly1305Cipher) Decrypt(header recordlayer.Header, in []byte) ([]byte, error) {
	err := header.Unmarshal(in)
	switch {
	case err != nil:
		return nil, err
	case header.ContentType == protocol.ContentTypeChangeCipherSpec:
		return in, nil
	case len(in) <= header.Size()+chachaTagLength:
		return nil, fmt.Errorf("ciphertext too short: %d <= %d", len(in), header.Size()+chachaTagLength)
	}

	nonce := computeNonce(c.remoteWriteIV, header.Epoch, header.SequenceNumber)
	out := in[header.Size():]
	additionalData := generateAEADAdditionalData(&header, len(out)-chachaTagLength)

	out, err = c.remoteCipher.Open(out[:0], nonce, out, additionalData)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errDecryptPacket, err)
	}

	return append(in[:header.Size()], out...), nil
}

type TLSEcdhePskWithChacha20Poly1305Sha256 struct {
	aead atomic.Value
}

func NewTLSEcdhePskWithChacha20Poly1305Sha256() *TLSEcdhePskWithChacha20Poly1305Sha256 {
	return &TLSEcdhePskWithChacha20Poly1305Sha256{}
}

func (c *TLSEcdhePskWithChacha20Poly1305Sha256) CertificateType() clientcertificate.Type {
	return clientcertificate.Type(0)
}

func (c *TLSEcdhePskWithChacha20Poly1305Sha256) KeyExchangeAlgorithm() dtls.CipherSuiteKeyExchangeAlgorithm {
	return dtls.CipherSuiteKeyExchangeAlgorithmPsk | dtls.CipherSuiteKeyExchangeAlgorithmEcdhe
}

func (c *TLSEcdhePskWithChacha20Poly1305Sha256) ECC() bool {
	return true
}

func (c *TLSEcdhePskWithChacha20Poly1305Sha256) ID() dtls.CipherSuiteID {
	return CipherSuiteID_CCAC
}

func (c *TLSEcdhePskWithChacha20Poly1305Sha256) String() string {
	return "TLS_ECDHE_PSK_WITH_CHACHA20_POLY1305_SHA256"
}

func (c *TLSEcdhePskWithChacha20Poly1305Sha256) HashFunc() func() hash.Hash {
	return sha256.New
}

func (c *TLSEcdhePskWithChacha20Poly1305Sha256) AuthenticationType() dtls.CipherSuiteAuthenticationType {
	return dtls.CipherSuiteAuthenticationTypePreSharedKey
}

func (c *TLSEcdhePskWithChacha20Poly1305Sha256) IsInitialized() bool {
	return c.aead.Load() != nil
}

func (c *TLSEcdhePskWithChacha20Poly1305Sha256) Init(masterSecret, clientRandom, serverRandom []byte, isClient bool) error {
	const (
		prfMacLen = 0
		prfKeyLen = 32
		prfIvLen  = 12
	)

	keys, err := prf.GenerateEncryptionKeys(
		masterSecret, clientRandom, serverRandom,
		prfMacLen, prfKeyLen, prfIvLen,
		c.HashFunc(),
	)
	if err != nil {
		return err
	}

	var aead *ChaCha20Poly1305Cipher
	if isClient {
		aead, err = NewChaCha20Poly1305Cipher(
			keys.ClientWriteKey, keys.ClientWriteIV,
			keys.ServerWriteKey, keys.ServerWriteIV,
		)
	} else {
		aead, err = NewChaCha20Poly1305Cipher(
			keys.ServerWriteKey, keys.ServerWriteIV,
			keys.ClientWriteKey, keys.ClientWriteIV,
		)
	}
	if err != nil {
		return err
	}

	c.aead.Store(aead)
	return nil
}

func (c *TLSEcdhePskWithChacha20Poly1305Sha256) Encrypt(pkt *recordlayer.RecordLayer, raw []byte) ([]byte, error) {
	aead, ok := c.aead.Load().(*ChaCha20Poly1305Cipher)
	if !ok {
		return nil, fmt.Errorf("%w: unable to encrypt", errCipherSuiteNotInit)
	}
	return aead.Encrypt(pkt, raw)
}

func (c *TLSEcdhePskWithChacha20Poly1305Sha256) Decrypt(h recordlayer.Header, raw []byte) ([]byte, error) {
	aead, ok := c.aead.Load().(*ChaCha20Poly1305Cipher)
	if !ok {
		return nil, fmt.Errorf("%w: unable to decrypt", errCipherSuiteNotInit)
	}
	return aead.Decrypt(h, raw)
}

func CustomCipherSuites() []dtls.CipherSuite {
	return []dtls.CipherSuite{
		NewTLSEcdhePskWithChacha20Poly1305Sha256(),
	}
}
