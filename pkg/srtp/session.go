package srtp

import (
	"net"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/srtp/v2"
)

type Session struct {
	Local  *Endpoint
	Remote *Endpoint

	OnReadRTP func(packet *rtp.Packet)

	Recv int // bytes recv
	Send int // bytes send

	conn net.PacketConn // local conn endpoint
	addr net.Addr       // remote addr
}

type Endpoint struct {
	Addr       string
	Port       uint16
	MasterKey  []byte
	MasterSalt []byte
	SSRC       uint32

	srtp *srtp.Context
}

func (e *Endpoint) Init() error {
	var profile srtp.ProtectionProfile

	switch len(e.MasterKey) {
	case 16:
		profile = srtp.ProtectionProfileAes128CmHmacSha1_80
		//case 32:
		//	return srtp.ProtectionProfileAes256CmHmacSha1_80
	}

	var err error
	e.srtp, err = srtp.CreateContext(e.MasterKey, e.MasterSalt, profile)
	return err
}

func (s *Session) WriteRTP(packet *rtp.Packet) (int, error) {
	b, err := packet.Marshal()
	if err != nil {
		return 0, err
	}

	if b, err = s.Local.srtp.EncryptRTP(nil, b, nil); err != nil {
		return 0, err
	}

	return s.conn.WriteTo(b, s.addr)
}

func (s *Session) ReadRTP(b []byte) {
	packet := &rtp.Packet{}

	b, err := s.Remote.srtp.DecryptRTP(nil, b, &packet.Header)
	if err != nil {
		return
	}

	if err = packet.Unmarshal(b); err != nil {
		return
	}

	if s.OnReadRTP != nil {
		s.OnReadRTP(packet)
	}
}

func (s *Session) ReadRTCP(b []byte) {
	header := &rtcp.Header{}
	b, err := s.Remote.srtp.DecryptRTCP(nil, b, header)
	if err != nil {
		return
	}

	if _, err = rtcp.Unmarshal(b); err != nil {
		return
	}

	if header.Type == rtcp.TypeSenderReport {
		_ = s.KeepAlive()
	}
}

func (s *Session) KeepAlive() error {
	rep := rtcp.ReceiverReport{SSRC: s.Local.SSRC}
	b, err := rep.Marshal()
	if err != nil {
		return err
	}

	if b, err = s.Local.srtp.EncryptRTCP(nil, b, nil); err != nil {
		return err
	}

	_, err = s.conn.WriteTo(b, s.addr)
	return err
}
