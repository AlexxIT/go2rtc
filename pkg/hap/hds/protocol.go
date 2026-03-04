package hds

import (
	"errors"
	"sync"
)

// HDS message types
const (
	ProtoDataSend = "dataSend"
	ProtoControl  = "control"

	TopicOpen  = "open"
	TopicData  = "data"
	TopicClose = "close"
	TopicAck   = "ack"
	TopicHello = "hello"

	StatusSuccess = 0
)

// Message represents an HDS application-level message
type Message struct {
	Protocol string
	Topic    string
	ID       int64
	IsEvent  bool
	Status   int64
	Body     map[string]any
}

// Session wraps an HDS encrypted connection with application-level protocol handling.
// HDS messages format: [1 byte header_length][opack header dict][opack message dict]
type Session struct {
	conn *Conn
	mu   sync.Mutex
	id   int64

	OnDataSendOpen  func(streamID int) error
	OnDataSendClose func(streamID int) error
}

func NewSession(conn *Conn) *Session {
	return &Session{conn: conn}
}

// ReadMessage reads and decodes an HDS application message
func (s *Session) ReadMessage() (*Message, error) {
	buf := make([]byte, 64*1024)
	n, err := s.conn.Read(buf)
	if err != nil {
		return nil, err
	}
	data := buf[:n]

	if len(data) < 2 {
		return nil, errors.New("hds: message too short")
	}

	headerLen := int(data[0])
	if len(data) < 1+headerLen {
		return nil, errors.New("hds: header truncated")
	}

	headerData := data[1 : 1+headerLen]
	bodyData := data[1+headerLen:]

	headerVal, err := OpackUnmarshal(headerData)
	if err != nil {
		return nil, err
	}
	header, ok := headerVal.(map[string]any)
	if !ok {
		return nil, errors.New("hds: header is not dict")
	}

	msg := &Message{
		Protocol: opackString(header["protocol"]),
	}

	if topic, ok := header["event"]; ok {
		msg.IsEvent = true
		msg.Topic = opackString(topic)
	} else if topic, ok := header["request"]; ok {
		msg.Topic = opackString(topic)
		msg.ID = opackInt(header["id"])
	} else if topic, ok := header["response"]; ok {
		msg.Topic = opackString(topic)
		msg.ID = opackInt(header["id"])
		msg.Status = opackInt(header["status"])
	}

	if len(bodyData) > 0 {
		bodyVal, err := OpackUnmarshal(bodyData)
		if err != nil {
			return nil, err
		}
		if m, ok := bodyVal.(map[string]any); ok {
			msg.Body = m
		}
	}

	return msg, nil
}

// WriteMessage sends an HDS application message
func (s *Session) WriteMessage(header, body map[string]any) error {
	headerBytes := OpackMarshal(header)
	bodyBytes := OpackMarshal(body)

	msg := make([]byte, 0, 1+len(headerBytes)+len(bodyBytes))
	msg = append(msg, byte(len(headerBytes)))
	msg = append(msg, headerBytes...)
	msg = append(msg, bodyBytes...)

	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.conn.Write(msg)
	return err
}

// WriteResponse sends a response to a request
func (s *Session) WriteResponse(protocol, topic string, id int64, status int, body map[string]any) error {
	header := map[string]any{
		"protocol": protocol,
		"response": topic,
		"id":       id,
		"status":   status,
	}
	if body == nil {
		body = map[string]any{}
	}
	return s.WriteMessage(header, body)
}

// WriteEvent sends an unsolicited event
func (s *Session) WriteEvent(protocol, topic string, body map[string]any) error {
	header := map[string]any{
		"protocol": protocol,
		"event":    topic,
	}
	if body == nil {
		body = map[string]any{}
	}
	return s.WriteMessage(header, body)
}

// WriteRequest sends a request
func (s *Session) WriteRequest(protocol, topic string, body map[string]any) (int64, error) {
	s.mu.Lock()
	s.id++
	id := s.id
	s.mu.Unlock()

	header := map[string]any{
		"protocol": protocol,
		"request":  topic,
		"id":       id,
	}
	if body == nil {
		body = map[string]any{}
	}
	return id, s.WriteMessage(header, body)
}

// SendMediaInit sends the fMP4 initialization segment (ftyp+moov)
func (s *Session) SendMediaInit(streamID int, initData []byte) error {
	return s.WriteEvent(ProtoDataSend, TopicData, map[string]any{
		"streamId": streamID,
		"packets":  1,
		"type":     "mediaInitialization",
		"data":     initData,
	})
}

// SendMediaFragment sends an fMP4 fragment (moof+mdat)
func (s *Session) SendMediaFragment(streamID int, fragment []byte, sequence int) error {
	return s.WriteEvent(ProtoDataSend, TopicData, map[string]any{
		"streamId":               streamID,
		"packets":                1,
		"type":                   "mediaFragment",
		"data":                   fragment,
		"dataSequenceNumber":     sequence,
		"isLastDataChunk":        true,
		"dataChunkSequenceNumber": 0,
	})
}

// Run processes incoming HDS messages in a loop
func (s *Session) Run() error {
	// Handle control/hello handshake
	msg, err := s.ReadMessage()
	if err != nil {
		return err
	}

	if msg.Protocol == ProtoControl && msg.Topic == TopicHello {
		if err := s.WriteResponse(ProtoControl, TopicHello, msg.ID, StatusSuccess, nil); err != nil {
			return err
		}
	}

	// Main message loop
	for {
		msg, err := s.ReadMessage()
		if err != nil {
			return err
		}

		if msg.Protocol != ProtoDataSend {
			continue
		}

		switch msg.Topic {
		case TopicOpen:
			streamID := int(opackInt(msg.Body["streamId"]))
			// Acknowledge the open request
			if err := s.WriteResponse(ProtoDataSend, TopicOpen, msg.ID, StatusSuccess, nil); err != nil {
				return err
			}
			if s.OnDataSendOpen != nil {
				if err := s.OnDataSendOpen(streamID); err != nil {
					return err
				}
			}

		case TopicClose:
			streamID := int(opackInt(msg.Body["streamId"]))
			// Acknowledge the close request
			if err := s.WriteResponse(ProtoDataSend, TopicClose, msg.ID, StatusSuccess, nil); err != nil {
				return err
			}
			if s.OnDataSendClose != nil {
				if err := s.OnDataSendClose(streamID); err != nil {
					return err
				}
			}

		case TopicAck:
			// Acknowledgement from controller, nothing to do
		}
	}
}

func (s *Session) Close() error {
	return s.conn.Close()
}

func opackString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func opackInt(v any) int64 {
	switch v := v.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	}
	return 0
}
