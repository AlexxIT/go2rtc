package tutk

import (
	"encoding/binary"
	"errors"
	"time"
)

// Channel 1 also carries a small reliable stream next to the two-way audio (verified on a Mi Home capture
// of an Aqara G2, which uses it for the SD card record list). After the 28-byte TUTK header every frame is
//
//	0   5a97c2f1  magic
//	4   0502      type: 0501 hello, 0541 hello ack, 0502 data, 0546 data ack
//	6   3700      body size
//	8   00000000  seq (data) / acked seq (data ack)
//	12  00000000
//	16  01050000  hello and ack: window (0x500 camera, 0x100 us); data: 0x100 camera, 0x501 request
//	20  body
//
// The camera sends its hello after a `0x0700 {"channel":1}` command; both sides ack every data frame by seq.

const ch1Magic = "\x5a\x97\xc2\xf1"

type ch1 struct {
	ready  chan struct{} // closed once the camera acked our hello
	data   chan []byte
	out    uint32
	hello  bool
	open   bool
	acked  chan struct{}
	hello1 chan struct{}
	camTag uint32
}

func (s *Session16) ch1Frame(typ uint16, seq uint32, tag uint32, body []byte) []byte {
	msg := s.Msg(uint16(msgHhrSize + 20 + len(body)))
	b := msg[msgHhrSize:]
	copy(b, ch1Magic)
	binary.LittleEndian.PutUint16(b[4:], typ)
	binary.LittleEndian.PutUint16(b[6:], uint16(len(body)))
	binary.LittleEndian.PutUint32(b[8:], seq)
	binary.LittleEndian.PutUint32(b[16:], tag)
	copy(b[20:], body)
	return msg
}

// handleCh1RUDP answers the camera's reliable-stream frames; false if cmd is not one.
func (s *Session16) handleCh1RUDP(cmd []byte) bool {
	if len(cmd) < 20 || string(cmd[:4]) != ch1Magic {
		return false
	}
	switch binary.LittleEndian.Uint16(cmd[4:]) {
	case 0x0501:
		if t := binary.LittleEndian.Uint32(cmd[16:]); t != s.ch1.camTag {
			s.ch1.camTag = t
		}
		_ = s.SessionWrite(1, s.ch1Frame(0x0541, 0, binary.LittleEndian.Uint32(cmd[16:]), nil))
		if ch := s.ch1.hello1; ch != nil {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
		if !s.ch1.hello {
			s.ch1.hello = true
			_ = s.SessionWrite(1, s.ch1Frame(0x0501, 0, 0x100, nil))
		}
	case 0x0541: // the camera took our hello
		if ch := s.ch1.acked; ch != nil {
			s.ch1.acked = nil
			close(ch)
		}
		if !s.ch1.open {
			s.ch1.open = true
			close(s.ch1.ready)
		}
	case 0x0502:
		n := int(binary.LittleEndian.Uint16(cmd[6:]))
		_ = s.SessionWrite(1, s.ch1Frame(0x0546, binary.LittleEndian.Uint32(cmd[8:]), 0x500, nil))
		if len(cmd) >= 20+n {
			select {
			case s.ch1.data <- append([]byte{}, cmd[20:20+n]...):
			default:
			}
		}
	}
	return true
}

// Ch1Request sends one JSON request on the reliable stream and returns the reply: the camera streams data
// frames back-to-back, so the reply is whatever arrived before it went quiet. Needs `0x0700 {"channel":1}`
// sent first (the camera then opens the stream).
func (c *Conn) Ch1Request(js string) ([]byte, error) {
	s, ok := c.session.(*Session16)
	if !ok {
		return nil, errors.New("tutk: no reliable channel on this session")
	}
	select {
	case <-s.ch1.ready:
	case <-time.After(5 * time.Second):
		return nil, errors.New("tutk: reliable channel not open")
	}
	// Mi Home sends its request right after acking one of the camera's hellos (they come every second);
	// a request at any other time goes unanswered.
	s.ch1.hello1 = make(chan struct{}, 1)
	select {
	case <-s.ch1.hello1:
	case <-time.After(3 * time.Second):
		return nil, errors.New("tutk: no hello from the camera")
	}
	for len(s.ch1.data) > 0 {
		<-s.ch1.data
	}

	body := make([]byte, 8+len(js))
	binary.LittleEndian.PutUint32(body, 1)
	binary.LittleEndian.PutUint32(body[4:], uint32(len(js)))
	copy(body[8:], js)
	tag := s.ch1.camTag | 1 // the request tag follows the camera's hello tag (0x500 -> 0x501; a fixed 0x501 gets no answer)
	if err := s.SessionWrite(1, s.ch1Frame(0x0502, s.ch1.out, tag, body)); err != nil {
		return nil, err
	}
	s.ch1.out++

	var reply []byte
	idle := 5 * time.Second // the first frame can take a while, then they come in a burst
	for {
		select {
		case b := <-s.ch1.data:
			reply = append(reply, b...)
			idle = 600 * time.Millisecond
		case <-time.After(idle):
			if len(reply) == 0 {
				return nil, errors.New("tutk: no reply")
			}
			return reply, nil
		}
	}
}
