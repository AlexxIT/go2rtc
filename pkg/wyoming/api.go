package wyoming

import (
	"bufio"
	"encoding/json"
	"io"
	"net"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type API struct {
	conn net.Conn
	rd   *bufio.Reader
}

func DialAPI(address string) (*API, error) {
	conn, err := net.DialTimeout("tcp", address, core.ConnDialTimeout)
	if err != nil {
		return nil, err
	}

	return NewAPI(conn), nil
}

const Version = "1.5.4"

func NewAPI(conn net.Conn) *API {
	return &API{conn: conn, rd: bufio.NewReader(conn)}
}

func (w *API) WriteEvent(evt *Event) (err error) {
	hdr := EventHeader{
		Type:          evt.Type,
		Version:       Version,
		DataLength:    len(evt.Data),
		PayloadLength: len(evt.Payload),
	}

	buf, err := json.Marshal(hdr)
	if err != nil {
		return err
	}

	buf = append(buf, '\n')
	buf = append(buf, evt.Data...)
	buf = append(buf, evt.Payload...)

	_, err = w.conn.Write(buf)
	return err
}

func (w *API) ReadEvent() (*Event, error) {
	data, err := w.rd.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	var hdr EventHeader
	if err = json.Unmarshal(data, &hdr); err != nil {
		return nil, err
	}

	evt := Event{Type: hdr.Type}

	if hdr.DataLength > 0 {
		data = make([]byte, hdr.DataLength)
		if _, err = io.ReadFull(w.rd, data); err != nil {
			return nil, err
		}
		evt.Data = string(data)
	}

	if hdr.PayloadLength > 0 {
		evt.Payload = make([]byte, hdr.PayloadLength)
		if _, err = io.ReadFull(w.rd, evt.Payload); err != nil {
			return nil, err
		}
	}

	return &evt, nil
}

func (w *API) Close() error {
	return w.conn.Close()
}

type Event struct {
	Type    string
	Data    string
	Payload []byte
}

type EventHeader struct {
	Type          string `json:"type"`
	Version       string `json:"version"`
	DataLength    int    `json:"data_length,omitempty"`
	PayloadLength int    `json:"payload_length,omitempty"`
}
