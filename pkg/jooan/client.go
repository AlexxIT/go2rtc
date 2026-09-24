package jooan

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/tutk/dtls"
	piondtls "github.com/pion/dtls/v3"
)

const (
	ProtocolVersion byte   = 29
	SpeakerChannel  uint32 = 1
)

var SDKVersion = [4]byte{0x03, 0x06, 0x03, 0x04}

// Session is the narrow CAM720 operation set used by Producer.
type Session interface {
	AVClientStart(time.Duration) error
	HasTwoWayStreaming() bool
	AVTwoWayStart(uint32) error
	AVTwoWayStop(uint32) error
	AVSendAudioDataTwoWay(byte, []byte, uint32, uint32, uint8) error
	Close() error
}

type dialer func(Config) (Session, error)

var dialSession dialer = dial

func dial(config Config) (Session, error) {
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(config.Host, strconv.Itoa(config.port())))
	if err != nil {
		return nil, fmt.Errorf("jooan: resolve host: %w", err)
	}
	options := dtls.Options{
		OuterProtocolVersion: ProtocolVersion,
		SDKVersion:           SDKVersion,
		AVUsername:           config.Username,
		AVPassword:           config.Password,
		AVLoginSecurityMode:  2,
		CipherSuites:         []piondtls.CipherSuiteID{piondtls.TLS_PSK_WITH_AES_128_CBC_SHA256},
	}
	return dtls.DialDTLSWithOptions(addr.IP.String(), addr.Port, config.UID, "", "", false, options)
}

func newSession(config Config) (Session, error) {
	return dialSession(config)
}
