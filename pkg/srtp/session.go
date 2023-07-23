package srtp

import (
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
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
	Track *core.Receiver
	Recv  uint32

	Deadline *time.Timer

	lastSequence  uint32
	lastTimestamp uint32
	//lastPacket    *rtp.Packet
	lastTime time.Time
	jitter   float64
	//sequenceCycle uint16
	totalLost uint32
}

func (s *Session) LastTime() time.Time {
	return s.lastTime
}

func (s *Session) SetKeys(localKey, localSalt, remoteKey, remoteSalt []byte) (err error) {
	s.localCtx, err = srtp.CreateContext(localKey, localSalt, GuessProfile(localKey))
	if err != nil {
		return
	}
	s.remoteCtx, err = srtp.CreateContext(remoteKey, remoteSalt, GuessProfile(remoteKey))
	return
}

func (s *Session) HandleRTP(data []byte) (err error) {
	if data, err = s.remoteCtx.DecryptRTP(nil, data, nil); err != nil {
		return
	}

	if s.Track == nil {
		return
	}

	packet := &rtp.Packet{}
	if err = packet.Unmarshal(data); err != nil {
		return
	}

	if s.Deadline != nil {
		s.Deadline.Reset(core.ConnDeadline)
	}

	now := time.Now()

	// https://www.ietf.org/rfc/rfc3550.txt
	if s.lastTimestamp != 0 {
		delta := packet.SequenceNumber - uint16(s.lastSequence)

		// lost packet
		if delta > 1 {
			s.totalLost += uint32(delta - 1)
		}

		// D(i,j) = (Rj - Ri) - (Sj - Si) = (Rj - Sj) - (Ri - Si)
		dTime := now.Sub(s.lastTime).Seconds()*float64(s.Track.Codec.ClockRate) -
			float64(packet.Timestamp-s.lastTimestamp)
		if dTime < 0 {
			dTime = -dTime
		}
		// J(i) = J(i-1) + (|D(i-1,i)| - J(i-1))/16
		s.jitter += (dTime - s.jitter) / 16
	}

	// keeping cycles (overflow)
	s.lastSequence = s.lastSequence&0xFFFF0000 | uint32(packet.SequenceNumber)
	s.lastTimestamp = packet.Timestamp
	s.lastTime = now

	s.Track.WriteRTP(packet)

	return
}

func (s *Session) HandleRTCP(data []byte) (err error) {
	header := &rtcp.Header{}
	if data, err = s.remoteCtx.DecryptRTCP(nil, data, header); err != nil {
		return
	}

	if _, err = rtcp.Unmarshal(data); err != nil {
		return
	}

	if header.Type == rtcp.TypeSenderReport {
		err = s.KeepAlive()
	}

	return
}

func (s *Session) KeepAlive() (err error) {
	rep := rtcp.ReceiverReport{SSRC: s.LocalSSRC}

	if s.lastTimestamp > 0 {
		//log.Printf("[RTCP] ssrc=%d seq=%d lost=%d jit=%.2f", s.RemoteSSRC, s.lastSequence, s.totalLost, s.jitter)

		rep.Reports = []rtcp.ReceptionReport{{
			SSRC:               s.RemoteSSRC,
			LastSequenceNumber: s.lastSequence,
			LastSenderReport:   s.lastTimestamp,
			FractionLost:       0, // TODO
			TotalLost:          s.totalLost,
			Delay:              0, // send just after receive
			Jitter:             uint32(s.jitter),
		}}
	}

	// we can send empty receiver response, but should send it to hold the connection

	var data []byte
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
