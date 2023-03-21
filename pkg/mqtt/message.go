package mqtt

import (
	"io"
)

type Message struct {
	b []byte
}

// https://docs.oasis-open.org/mqtt/mqtt/v5.0/mqtt-v5.0.html
const (
	CONNECT   = 0x10
	CONNACK   = 0x20
	PUBLISH   = 0x30
	PUBACK    = 0x40
	SUBSCRIBE = 0x82
	SUBACK    = 0x90
	QOS1      = 0x02
)

func (m *Message) WriteByte(b byte) {
	m.b = append(m.b, b)
}

func (m *Message) WriteBytes(b []byte) {
	m.b = append(m.b, b...)
}

func (m *Message) WriteUint16(i uint16) {
	m.b = append(m.b, byte(i>>8), byte(i))
}

func (m *Message) WriteLen(i int) {
	for i > 0 {
		b := byte(i % 128)
		if i /= 128; i > 0 {
			b |= 0x80
		}
		m.WriteByte(b)
	}
}

func (m *Message) WriteString(s string) {
	m.WriteUint16(uint16(len(s)))
	m.b = append(m.b, s...)
}

func (m *Message) Bytes() []byte {
	return m.b
}

const (
	flagCleanStart = 0x02
	flagUsername   = 0x80
	flagPassword   = 0x40
)

func NewConnect(clientID, username, password string) *Message {
	m := &Message{}
	m.WriteByte(CONNECT)
	m.WriteLen(16 + len(clientID) + len(username) + len(password))

	m.WriteString("MQTT")
	m.WriteByte(4) // MQTT version
	m.WriteByte(flagCleanStart | flagUsername | flagPassword)
	m.WriteUint16(30) // keepalive

	m.WriteString(clientID)
	m.WriteString(username)
	m.WriteString(password)
	return m
}

func NewSubscribe(mid uint16, topic string, qos byte) *Message {
	m := &Message{}
	m.WriteByte(SUBSCRIBE)
	m.WriteLen(5 + len(topic))

	m.WriteUint16(mid)
	m.WriteString(topic)
	m.WriteByte(qos)
	return m
}

func NewPublish(topic string, payload []byte) *Message {
	m := &Message{}
	m.WriteByte(PUBLISH)
	m.WriteLen(2 + len(topic) + len(payload))

	m.WriteString(topic)
	m.WriteBytes(payload)
	return m
}

func NewPublishQOS1(mid uint16, topic string, payload []byte) *Message {
	m := &Message{}
	m.WriteByte(PUBLISH | QOS1)
	m.WriteLen(4 + len(topic) + len(payload))

	m.WriteString(topic)
	m.WriteUint16(mid)
	m.WriteBytes(payload)
	return m
}

func ReadLen(r io.Reader) (uint32, error) {
	var i uint32
	var shift byte

	b := []byte{0x80}
	for b[0]&0x80 != 0 {
		if _, err := r.Read(b); err != nil {
			return 0, err
		}

		i += uint32(b[0]&0x7F) << shift
		shift += 7
	}

	return i, nil
}
