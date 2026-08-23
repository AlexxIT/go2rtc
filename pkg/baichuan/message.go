package baichuan

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	wireMagic = 0x0abcdef0

	classLegacy = 0x6514
	classModern = 0x6614
	classOffset = 0x6414
	classAlt    = 0x0000

	commandLogin       = 1
	commandPreview     = 3
	commandStopPreview = 4
	commandTalkAbility = 10
	commandStopTalk    = 11
	commandDeviceInfo  = 80
	commandPing        = 93
	commandAbility     = 151
	commandTalkConfig  = 201
	commandTalkData    = 202
)

type header struct {
	Command       uint32
	BodyLen       uint32
	Channel       uint8
	Stream        uint8
	Sequence      uint16
	ResponseCode  uint16
	Class         uint16
	PayloadOffset uint32
}

func (h header) hasOffset() bool {
	return h.Class == classOffset || h.Class == classAlt
}

type frame struct {
	header header
	body   []byte
}

type message struct {
	header        header
	extension     []byte
	payload       []byte
	binary        bool
	encrypt       int
	hasEncryptLen bool
	checkPos      int
	hasCheckPos   bool
}

type request struct {
	command   uint32
	channel   uint8
	stream    uint8
	sequence  uint16
	class     uint16
	extension []byte
	payload   []byte
	binary    bool
	forceBC   bool
}

func readFrame(r io.Reader, limits Limits) (frame, error) {
	var base [20]byte
	if _, err := io.ReadFull(r, base[:]); err != nil {
		return frame{}, err
	}
	if magic := binary.LittleEndian.Uint32(base[:4]); magic != wireMagic {
		return frame{}, fmt.Errorf("baichuan: invalid message magic %#x", magic)
	}

	h := header{
		Command:      binary.LittleEndian.Uint32(base[4:8]),
		BodyLen:      binary.LittleEndian.Uint32(base[8:12]),
		Channel:      base[12],
		Stream:       base[13],
		Sequence:     binary.LittleEndian.Uint16(base[14:16]),
		ResponseCode: binary.LittleEndian.Uint16(base[16:18]),
		Class:        binary.LittleEndian.Uint16(base[18:20]),
	}
	if h.Class != classLegacy && h.Class != classModern && h.Class != classOffset && h.Class != classAlt {
		return frame{}, fmt.Errorf("baichuan: unsupported message class %#x", h.Class)
	}
	if h.BodyLen > limits.MaxBody {
		return frame{}, fmt.Errorf("baichuan: message body %d exceeds limit %d", h.BodyLen, limits.MaxBody)
	}
	if h.hasOffset() {
		var offset [4]byte
		if _, err := io.ReadFull(r, offset[:]); err != nil {
			return frame{}, err
		}
		h.PayloadOffset = binary.LittleEndian.Uint32(offset[:])
		if h.PayloadOffset > h.BodyLen || h.PayloadOffset > limits.MaxExtension {
			return frame{}, fmt.Errorf("baichuan: invalid payload offset %d for body %d", h.PayloadOffset, h.BodyLen)
		}
	}
	payload := h.BodyLen - h.PayloadOffset
	if limit := payloadLimit(h.Command, limits); payload > limit {
		return frame{}, fmt.Errorf("baichuan: command %d payload %d exceeds limit %d", h.Command, payload, limit)
	}

	body := make([]byte, h.BodyLen)
	if _, err := io.ReadFull(r, body); err != nil {
		return frame{}, err
	}
	return frame{header: h, body: body}, nil
}

func payloadLimit(command uint32, limits Limits) uint32 {
	var limit uint32
	switch command {
	case commandLogin:
		limit = maxNonceBody
	case commandTalkAbility:
		limit = maxTalkAbilityBody
	case commandDeviceInfo:
		limit = maxDeviceInfoBody
	case commandAbility:
		limit = maxCapabilityBody
	default:
		return limits.MaxBody
	}
	if limit > limits.MaxBody {
		return limits.MaxBody
	}
	return limit
}

func decodeFrame(value frame, cipher cipherState, binaryHint bool) (message, error) {
	if value.header.PayloadOffset > uint32(len(value.body)) {
		return message{}, fmt.Errorf("baichuan: invalid payload offset %d for body %d",
			value.header.PayloadOffset, len(value.body))
	}
	offset := int(value.header.PayloadOffset)
	extensionBody := value.body[:offset]
	cipher.decryptInPlace(value.header.Channel, extensionBody)
	extensionMeta, err := parseExtension(extensionBody)
	if err != nil {
		return message{}, err
	}
	binaryPayload := binaryHint || extensionMeta.BinaryData != nil && *extensionMeta.BinaryData == 1

	payload := value.body[offset:]
	encryptLen := 0
	if !binaryPayload {
		cipher.decryptInPlace(value.header.Channel, payload)
	} else {
		if extensionMeta.EncryptLen != nil {
			n := *extensionMeta.EncryptLen
			if n < 0 || n > len(payload) {
				return message{}, fmt.Errorf("baichuan: invalid encrypted prefix %d for payload %d", n, len(payload))
			}
			if n > 0 {
				cipher.decryptInPlace(value.header.Channel, payload[:n])
			}
			encryptLen = n
		}
	}
	msg := message{
		header: value.header, extension: extensionBody, payload: payload,
		binary: binaryPayload, encrypt: encryptLen, hasEncryptLen: extensionMeta.EncryptLen != nil,
	}
	if extensionMeta.CheckPos != nil {
		msg.checkPos = *extensionMeta.CheckPos
		msg.hasCheckPos = true
	}
	return msg, nil
}

func hasMediaMagic(b []byte) bool {
	return len(b) >= 4 && knownMediaMagic(binary.LittleEndian.Uint32(b))
}

func encodeRequest(value request, limits Limits, cipher cipherState) ([]byte, error) {
	if value.class != classLegacy && value.class != classModern && value.class != classOffset && value.class != classAlt {
		return nil, fmt.Errorf("baichuan: unsupported request class %#x", value.class)
	}
	if value.forceBC {
		cipher.mode = encryptionBC
	}
	bodyLen := uint64(len(value.extension)) + uint64(len(value.payload))
	if bodyLen > uint64(limits.MaxBody) || len(value.extension) > int(limits.MaxExtension) {
		return nil, fmt.Errorf("baichuan: request exceeds configured limits")
	}

	headerLen := 20
	if value.class == classOffset || value.class == classAlt {
		headerLen = 24
	}
	packet := make([]byte, headerLen+int(bodyLen))
	binary.LittleEndian.PutUint32(packet[0:4], wireMagic)
	binary.LittleEndian.PutUint32(packet[4:8], value.command)
	binary.LittleEndian.PutUint32(packet[8:12], uint32(bodyLen))
	packet[12] = value.channel
	packet[13] = value.stream
	binary.LittleEndian.PutUint16(packet[14:16], value.sequence)
	if value.class == classLegacy && value.command == commandLogin && len(value.payload) == 0 {
		binary.LittleEndian.PutUint16(packet[16:18], 0xdc12)
	}
	binary.LittleEndian.PutUint16(packet[18:20], value.class)
	if headerLen == 24 {
		binary.LittleEndian.PutUint32(packet[20:24], uint32(len(value.extension)))
	}
	extension := packet[headerLen : headerLen+len(value.extension)]
	payload := packet[headerLen+len(value.extension):]
	copy(extension, value.extension)
	copy(payload, value.payload)
	if value.class != classLegacy {
		cipher.cryptInPlace(value.channel, extension, true)
		if !value.binary {
			cipher.cryptInPlace(value.channel, payload, true)
		}
	}
	return packet, nil
}

func responseError(h header) error {
	if _, ok := negotiatedEncryption(h.ResponseCode); ok {
		return nil
	}
	switch h.ResponseCode {
	case 0, 200, 201, 300:
		return nil
	default:
		return &StatusError{Command: h.Command, Code: h.ResponseCode}
	}
}

type StatusError struct {
	Command uint32
	Code    uint16
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("baichuan: command %d failed with status %d", e.Command, e.Code)
}
