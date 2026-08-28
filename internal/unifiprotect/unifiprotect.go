package unifiprotect

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

const (
	defaultTLSCert = "go2rtc-unifi-protect.crt"
	defaultTLSKey  = "go2rtc-unifi-protect.key"
)

type config struct {
	Listen      string `yaml:"listen"`
	MediaListen string `yaml:"media_listen"`
	MediaHost   string `yaml:"media_host"`
	TLSCert     string `yaml:"tls_cert"`
	TLSKey      string `yaml:"tls_key"`
}

var (
	log     zerolog.Logger
	manager *Manager
)

func Init() {
	log = app.GetLogger("unifi_protect")
	streams.HandleFunc("unifi-protect", func(source string) (core.Producer, error) {
		if manager == nil {
			return nil, errors.New("unifi-protect: unifi_protect is not configured")
		}
		return manager.open(source)
	})

	var cfg struct {
		Mod *config `yaml:"unifi_protect"`
	}
	app.LoadConfig(&cfg)
	if cfg.Mod == nil {
		return
	}
	if cfg.Mod.Listen == "" {
		cfg.Mod.Listen = ":7442"
	}
	if cfg.Mod.MediaListen == "" {
		cfg.Mod.MediaListen = ":7550"
	}

	cert, err := loadCertificate(cfg.Mod.TLSCert, cfg.Mod.TLSKey)
	if err != nil {
		log.Error().Err(err).Msg("[unifi-protect] TLS certificate")
		return
	}

	var controlListener net.Listener
	if cfg.Mod.Listen == cfg.Mod.MediaListen {
		listener, err := net.Listen("tcp", cfg.Mod.Listen)
		if err != nil {
			log.Error().Err(err).Msg("[unifi-protect] shared listen")
			return
		}
		mediaPort := listener.Addr().(*net.TCPAddr).Port
		manager = newManager(cfg.Mod.MediaHost, mediaPort, app.Version, certificateUUID(cert))
		shared := newSharedListener(listener)
		controlListener = shared
		log.Info().Str("addr", listener.Addr().String()).Msg("[unifi-protect] shared listen")
		go shared.serve(manager)
	} else {
		mediaListener, err := net.Listen("tcp", cfg.Mod.MediaListen)
		if err != nil {
			log.Error().Err(err).Msg("[unifi-protect] media listen")
			return
		}
		listener, err := net.Listen("tcp", cfg.Mod.Listen)
		if err != nil {
			_ = mediaListener.Close()
			log.Error().Err(err).Msg("[unifi-protect] control listen")
			return
		}
		controlListener = listener
		mediaPort := mediaListener.Addr().(*net.TCPAddr).Port
		manager = newManager(cfg.Mod.MediaHost, mediaPort, app.Version, certificateUUID(cert))
		log.Info().Str("addr", controlListener.Addr().String()).Msg("[unifi-protect] control listen")
		log.Info().Str("addr", mediaListener.Addr().String()).Msg("[unifi-protect] media listen")
		go manager.serveMedia(mediaListener)
	}

	go func() {
		server := &http.Server{
			Handler:           newController(manager),
			ReadHeaderTimeout: 5 * time.Second,
		}
		tlsConfig := &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{cert},
		}
		if err := server.Serve(tls.NewListener(controlListener, tlsConfig)); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("[unifi-protect] control serve")
		}
	}()
}

func loadCertificate(certValue, keyValue string) (tls.Certificate, error) {
	if certValue == "" && keyValue == "" {
		dir := "."
		if app.ConfigPath != "" {
			dir = filepath.Dir(app.ConfigPath)
		}
		certPath := filepath.Join(dir, defaultTLSCert)
		keyPath := filepath.Join(dir, defaultTLSKey)
		_, statErr := os.Stat(certPath)
		cert, err := loadOrCreateCertificate(certPath, keyPath)
		if err == nil && errors.Is(statErr, os.ErrNotExist) {
			log.Info().Str("cert", certPath).Str("key", keyPath).Msg("[unifi-protect] created TLS identity")
		}
		return cert, err
	}
	if certValue == "" || keyValue == "" {
		return tls.Certificate{}, errors.New("tls_cert and tls_key must be configured together")
	}
	if strings.Contains(certValue, "\n") || strings.Contains(keyValue, "\n") {
		return tls.X509KeyPair([]byte(certValue), []byte(keyValue))
	}
	return tls.LoadX509KeyPair(certValue, keyValue)
}

func loadOrCreateCertificate(certPath, keyPath string) (tls.Certificate, error) {
	certInfo, certErr := os.Stat(certPath)
	keyInfo, keyErr := os.Stat(keyPath)

	switch {
	case certErr == nil && keyErr == nil:
		if certInfo.IsDir() || keyInfo.IsDir() {
			return tls.Certificate{}, errors.New("TLS certificate path is a directory")
		}
		return tls.LoadX509KeyPair(certPath, keyPath)
	case errors.Is(certErr, os.ErrNotExist) && errors.Is(keyErr, os.ErrNotExist):
		// Generate the identity only when neither half exists. A camera pins the
		// exact certificate during set-inform, so silently replacing a damaged or
		// partially missing identity would leave adopted cameras disconnected.
	case certErr != nil && !errors.Is(certErr, os.ErrNotExist):
		return tls.Certificate{}, certErr
	case keyErr != nil && !errors.Is(keyErr, os.ErrNotExist):
		return tls.Certificate{}, keyErr
	default:
		return tls.Certificate{}, fmt.Errorf("unifi-protect: incomplete TLS identity: both %s and %s are required", certPath, keyPath)
	}

	certPEM, keyPEM, err := createCertificate()
	if err != nil {
		return tls.Certificate{}, err
	}
	if err = writeFileAtomic(keyPath, keyPEM, 0600); err != nil {
		return tls.Certificate{}, err
	}
	if err = writeFileAtomic(certPath, certPEM, 0644); err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(certPEM, keyPEM)
}

func createCertificate() (certPEM, keyPEM []byte, err error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, err
	}
	if serial.Sign() == 0 {
		serial.SetInt64(1)
	}

	now := time.Now()
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "go2rtc"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	return certPEM, keyPEM, nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".unifi-protect-tls-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		_ = f.Close()
		_ = os.Remove(tmp)
	}()
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func certificateUUID(cert tls.Certificate) string {
	if len(cert.Certificate) == 0 {
		return uuid.Nil.String()
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, cert.Certificate[0]).String()
}
