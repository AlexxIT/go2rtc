package hap

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"github.com/brutella/hap/characteristic"
	"github.com/brutella/hap/tlv8"
	"io"
	"net/http"
)

type Character struct {
	AID         int         `json:"aid,omitempty"`
	IID         int         `json:"iid"`
	Type        string      `json:"type,omitempty"`
	Format      string      `json:"format,omitempty"`
	Value       interface{} `json:"value,omitempty"`
	Event       interface{} `json:"ev,omitempty"`
	Perms       []string    `json:"perms,omitempty"`
	Description string      `json:"description,omitempty"`
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

	for w, _ := range c.listeners {
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
func (c *Character) Set(v interface{}) (err error) {
	if err = c.Write(v); err != nil {
		return
	}
	return c.NotifyListeners(nil)
}

// Write new value with right format
func (c *Character) Write(v interface{}) (err error) {
	switch c.Format {
	case characteristic.FormatTLV8:
		var data []byte
		if data, err = tlv8.Marshal(v); err != nil {
			return
		}
		c.Value = base64.StdEncoding.EncodeToString(data)

	case characteristic.FormatBool:
		switch v.(type) {
		case bool:
			c.Value = v.(bool)
		case float64:
			c.Value = v.(float64) != 0
		}
	}
	return
}

// ReadTLV8 value to right struct
func (c *Character) ReadTLV8(v interface{}) (err error) {
	var data []byte
	if data, err = base64.StdEncoding.DecodeString(c.Value.(string)); err != nil {
		return
	}
	return tlv8.Unmarshal(data, v)
}

func (c *Character) ReadBool() bool {
	return c.Value.(bool)
}
