package homekit

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"sync"
)

// Credentials holds CMAF ingest identity material provisioned over HAP
type Credentials struct {
	mu sync.RWMutex

	privKey    *ecdsa.PrivateKey
	clientCert []byte // DER leaf
	caCert     []byte // DER CA

	// content keys from Camera Key Management, keyed by key number
	keys  map[uint64][]byte
	keyID uint64

	// publishing point
	publishURL string
	serverCAs  [][]byte // DER certs
}

func NewCredentials() *Credentials {
	return &Credentials{keys: make(map[uint64][]byte)}
}

// HandleCSR builds a DER CSR and signs the controller nonce with the same key
func (c *Credentials) HandleCSR(nonce []byte) (csrDER, nonceSig []byte, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.privKey == nil {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		c.privKey = key
	}

	template := x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "go2rtc-hksv"},
	}
	csrDER, err = x509.CreateCertificateRequest(rand.Reader, &template, c.privKey)
	if err != nil {
		return nil, nil, err
	}

	sum := sha256.Sum256(nonce)
	nonceSig, err = ecdsa.SignASN1(rand.Reader, c.privKey, sum[:])
	if err != nil {
		return nil, nil, err
	}
	return csrDER, nonceSig, nil
}

// InstallClientCertificate stores the issued leaf and CA (DER)
func (c *Credentials) InstallClientCertificate(clientDER, caDER []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clientCert = append([]byte(nil), clientDER...)
	c.caCert = append([]byte(nil), caDER...)
}

// NeedsUpdate is true when no client certificate is installed
func (c *Credentials) NeedsUpdate() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.clientCert) == 0
}

// SetKey stores a content key and marks it current
func (c *Credentials) SetKey(key []byte, number uint64) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keys[number] = append([]byte(nil), key...)
	c.keyID = number
	return number
}

// KeyID returns the current content key id
func (c *Credentials) KeyID() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.keyID
}

// CurrentKey returns the active content key bytes (may be nil)
func (c *Credentials) CurrentKey() (id uint64, key []byte) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if k, ok := c.keys[c.keyID]; ok {
		return c.keyID, append([]byte(nil), k...)
	}
	return c.keyID, nil
}

// SetPublishingPoint stores the CMAF ingest URL and server CA list
func (c *Credentials) SetPublishingPoint(url string, serverCAs [][]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.publishURL = url
	c.serverCAs = nil
	for _, ca := range serverCAs {
		c.serverCAs = append(c.serverCAs, append([]byte(nil), ca...))
	}
}

// PublishingPoint returns the configured URL
func (c *Credentials) PublishingPoint() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.publishURL
}

// TLSConfig builds an mTLS client config for CMAF ingest
// Returns nil, err if client cert is missing
func (c *Credentials) TLSConfig() (*tls.Config, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.clientCert) == 0 || c.privKey == nil {
		return nil, errNoClientCert
	}

	leaf, err := x509.ParseCertificate(c.clientCert)
	if err != nil {
		return nil, err
	}

	tlsCert := tls.Certificate{
		Certificate: [][]byte{c.clientCert},
		PrivateKey:  c.privKey,
		Leaf:        leaf,
	}
	if len(c.caCert) > 0 {
		tlsCert.Certificate = append(tlsCert.Certificate, c.caCert)
	}

	roots := x509.NewCertPool()
	for _, der := range c.serverCAs {
		if cert, err := x509.ParseCertificate(der); err == nil {
			roots.AddCert(cert)
		}
	}
	// Also trust the provisioned CA as a root when present
	if len(c.caCert) > 0 {
		if cert, err := x509.ParseCertificate(c.caCert); err == nil {
			roots.AddCert(cert)
		}
	}

	cfg := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS12,
	}
	if len(roots.Subjects()) > 0 { //nolint:staticcheck // Subjects still fine for emptiness check
		cfg.RootCAs = roots
	}
	return cfg, nil
}

var errNoClientCert = errString("homekit: CMAF client certificate not provisioned")

type errString string

func (e errString) Error() string { return string(e) }
