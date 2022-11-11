package hap

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const DeviceAID = 1 // TODO: fix someday

func GenerateID(name string) string {
	sum := sha512.Sum512([]byte(name))
	return fmt.Sprintf(
		"%02X:%02X:%02X:%02X:%02X:%02X",
		sum[0], sum[1], sum[2], sum[3], sum[4], sum[5],
	)
}

func GenerateUUID() string {
	//12345678-9012-3456-7890-123456789012
	data := make([]byte, 16)
	_, _ = rand.Read(data)
	s := hex.EncodeToString(data)
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}

type PairVerifyPayload struct {
	Method        byte   `tlv8:"0,optional"`
	Identifier    string `tlv8:"1,optional"`
	PublicKey     []byte `tlv8:"3,optional"`
	EncryptedData []byte `tlv8:"5,optional"`
	State         byte   `tlv8:"6,optional"`
	Status        byte   `tlv8:"7,optional"`
	Signature     []byte `tlv8:"10,optional"`
}

//func (c *Character) Unmarshal(value interface{}) error {
//	switch c.Format {
//	case characteristic.FormatTLV8:
//		data, err := base64.StdEncoding.DecodeString(c.Value.(string))
//		if err != nil {
//			return err
//		}
//		return tlv8.Unmarshal(data, value)
//	}
//	return nil
//}

//func (c *Character) Marshal(value interface{}) error {
//	switch c.Format {
//	case characteristic.FormatTLV8:
//		data, err := tlv8.Marshal(value)
//		if err != nil {
//			return err
//		}
//		c.Value = base64.StdEncoding.EncodeToString(data)
//	}
//	return nil
//}

func (c *Character) String() string {
	data, err := json.Marshal(c)
	if err != nil {
		return "ERROR"
	}
	return string(data)
}

func UnmarshalEvent(res *http.Response) (char *Character, err error) {
	var data []byte
	if data, err = io.ReadAll(res.Body); err != nil {
		return
	}

	ch := Characters{}
	if err = json.Unmarshal(data, &ch); err != nil {
		return
	}

	if len(ch.Characters) > 1 {
		panic("not implemented")
	}

	char = ch.Characters[0]
	return
}
