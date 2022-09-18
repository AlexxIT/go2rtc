package srtp

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/srtp/v2"
)

type Session struct {
	LocalSSRC  uint32 // outgoing SSRC
	RemoteSSRC uint32 // incoming SSRC

	localCtx  *srtp.Context // write context
	remoteCtx *srtp.Context // read context

	Write func(b []byte) (int, error)
	Track *streamer.Track
}

func (s *Session) SetKeys(
	localKey, localSalt, remoteKey, remoteSalt []byte,
) (err error) {
	if s.localCtx, err = srtp.CreateContext(
		localKey, localSalt, GuessProfile(localKey),
	); err != nil {
		return
	}
	s.remoteCtx, err = srtp.CreateContext(
		remoteKey, remoteSalt, GuessProfile(remoteKey),
	)
	return
}

func (s *Session) HandleRTP(data []byte) (err error) {
	if data, err = s.remoteCtx.DecryptRTP(nil, data, nil); err != nil {
		return
	}

	packet := &rtp.Packet{}
	if err = packet.Unmarshal(data); err != nil {
		return
	}

	_ = s.Track.WriteRTP(packet)
	//s.Output(core.RTP{Channel: s.Channel, Packet: packet})

	return
}

func (s *Session) HandleRTCP(data []byte) (err error) {
	header := &rtcp.Header{}
	if data, err = s.remoteCtx.DecryptRTCP(nil, data, header); err != nil {
		return
	}

	var packets []rtcp.Packet
	if packets, err = rtcp.Unmarshal(data); err != nil {
		return
	}

	_ = packets
	//s.Output(core.RTCP{Channel: s.Channel + 1, Header: header, Packets: packets})

	if header.Type == rtcp.TypeSenderReport {
		err = s.KeepAlive()
	}

	return
}

func (s *Session) KeepAlive() (err error) {
	var data []byte
	// we can send empty receiver response, but should send it to hold the connection
	rep := rtcp.ReceiverReport{SSRC: s.LocalSSRC}
	if data, err = rep.Marshal(); err != nil {
		return
	}

	if data, err = s.localCtx.EncryptRTCP(nil, data, nil); err != nil {
		return
	}

	_, err = s.Write(data)

	return
}

func GuessProfile(masterKey []byte) srtp.ProtectionProfile {
	switch len(masterKey) {
	case 16:
		return srtp.ProtectionProfileAes128CmHmacSha1_80
	//case 32:
	//	return srtp.ProtectionProfileAes256CmHmacSha1_80
	}
	return 0
}
