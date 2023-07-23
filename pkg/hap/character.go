package hap

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
)

type Character struct {
	AID         int      `json:"aid,omitempty"`
	IID         int      `json:"iid"`
	Type        string   `json:"type,omitempty"`
	Format      string   `json:"format,omitempty"`
	Value       any      `json:"value,omitempty"`
	Event       any      `json:"ev,omitempty"`
	Perms       []string `json:"perms,omitempty"`
	Description string   `json:"description,omitempty"`
	//MaxDataLen int      `json:"maxDataLen"`

	listeners map[io.Writer]bool
}

func (c *Character) AddListener(w io.Writer) {
	// TODO: sync.Mutex
	if c.listeners == nil {
		c.listeners = map[io.Writer]bool{}
	}
	c.listeners[w] = true
}

func (c *Character) RemoveListener(w io.Writer) {
	delete(c.listeners, w)

	if len(c.listeners) == 0 {
		c.listeners = nil
	}
}

func (c *Character) NotifyListeners(ignore io.Writer) error {
	if c.listeners == nil {
		return nil
	}

	data, err := c.GenerateEvent()
	if err != nil {
		return err
	}

	for w := range c.listeners {
		if w == ignore {
			continue
		}
		if _, err = w.Write(data); err != nil {
			// error not a problem - just remove listener
			c.RemoveListener(w)
		}
	}

	return nil
}

// GenerateEvent with raw HTTP headers
func (c *Character) GenerateEvent() (data []byte, err error) {
	chars := Characters{
		Characters: []*Character{{AID: DeviceAID, IID: c.IID, Value: c.Value}},
	}
	if data, err = json.Marshal(chars); err != nil {
		return
	}

	res := http.Response{
		StatusCode:    http.StatusOK,
		ProtoMajor:    1,
		ProtoMinor:    0,
		Header:        http.Header{"Content-Type": []string{MimeJSON}},
		ContentLength: int64(len(data)),
		Body:          io.NopCloser(bytes.NewReader(data)),
	}

	buf := bytes.NewBuffer([]byte{0})
	if err = res.Write(buf); err != nil {
		return
	}
	copy(buf.Bytes(), "EVENT")

	return buf.Bytes(), err
}

// Set new value and NotifyListeners
func (c *Character) Set(v any) (err error) {
	if err = c.Write(v); err != nil {
		return
	}
	return c.NotifyListeners(nil)
}

// Write new value with right format
func (c *Character) Write(v any) (err error) {
	switch c.Format {
	case "tlv8":
		c.Value, err = tlv8.MarshalBase64(v)

	case "bool":
		switch v := v.(type) {
		case bool:
			c.Value = v
		case float64:
			c.Value = v != 0
		}
	}
	return
}

// ReadTLV8 value to right struct
func (c *Character) ReadTLV8(v any) (err error) {
	return tlv8.UnmarshalBase64(c.Value.(string), v)
}

func (c *Character) ReadBool() bool {
	return c.Value.(bool)
}

func (c *Character) String() string {
	data, err := json.Marshal(c)
	if err != nil {
		return "ERROR"
	}
	return string(data)
}
