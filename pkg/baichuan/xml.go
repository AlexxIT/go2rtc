package baichuan

import (
	"encoding/xml"
	"fmt"
)

const (
	maxNonceBody       = 4 << 10
	maxNonceSize       = 128
	binaryExtensionXML = "<body><binaryData>1</binaryData></body>"
)

var binaryExtensionValue = 1

type loginEnvelope struct {
	XMLName xml.Name  `xml:"body"`
	User    loginUser `xml:"LoginUser"`
	Net     loginNet  `xml:"LoginNet"`
}

type loginUser struct {
	Version  string `xml:"version,attr"`
	Username string `xml:"userName"`
	Password string `xml:"password"`
	UserVer  int    `xml:"userVer"`
}

type loginNet struct {
	Version string `xml:"version,attr"`
	Type    string `xml:"type"`
	UDPPort int    `xml:"udpPort"`
}

type nonceEnvelope struct {
	Encryption *struct {
		Nonce string `xml:"nonce"`
	} `xml:"Encryption"`
}

type extension struct {
	BinaryData *int `xml:"binaryData"`
	EncryptLen *int `xml:"encryptLen"`
	CheckPos   *int `xml:"checkPos"`
}

type previewEnvelope struct {
	XMLName xml.Name `xml:"body"`
	Preview struct {
		Version string `xml:"version,attr"`
		Channel uint8  `xml:"channelId"`
		Handle  uint32 `xml:"handle"`
		Stream  Stream `xml:"streamType"`
	} `xml:"Preview"`
}

type stopPreviewEnvelope struct {
	XMLName xml.Name `xml:"body"`
	Preview struct {
		Version string `xml:"version,attr"`
		Channel uint8  `xml:"channelId"`
		Handle  uint32 `xml:"handle"`
	} `xml:"Preview"`
}

func marshalDocument(value any) ([]byte, error) {
	body, err := xml.Marshal(value)
	if err != nil {
		return nil, err
	}
	doc := make([]byte, 0, len(xml.Header)+len(body))
	doc = append(doc, xml.Header...)
	return append(doc, body...), nil
}

func buildLogin(username, password, nonce string) ([]byte, error) {
	return marshalDocument(loginEnvelope{
		User: loginUser{
			Version:  "1.1",
			Username: modernMD5(username + nonce),
			Password: modernMD5(password + nonce),
			UserVer:  1,
		},
		Net: loginNet{Version: "1.1", Type: "LAN"},
	})
}

func parseNonce(body []byte) (string, error) {
	if len(body) > maxNonceBody {
		return "", fmt.Errorf("login nonce XML exceeds %d bytes", maxNonceBody)
	}
	var value nonceEnvelope
	if err := xml.Unmarshal(body, &value); err != nil {
		return "", fmt.Errorf("decode nonce XML: %w", err)
	}
	if value.Encryption == nil || value.Encryption.Nonce == "" {
		return "", fmt.Errorf("nonce missing from login response")
	}
	if len(value.Encryption.Nonce) > maxNonceSize {
		return "", fmt.Errorf("login nonce exceeds %d bytes", maxNonceSize)
	}
	return value.Encryption.Nonce, nil
}

func parseExtension(body []byte) (extension, error) {
	if len(body) == 0 {
		return extension{}, nil
	}
	if string(body) == binaryExtensionXML {
		return extension{BinaryData: &binaryExtensionValue}, nil
	}
	var value extension
	if err := xml.Unmarshal(body, &value); err != nil {
		return value, fmt.Errorf("decode extension XML: %w", err)
	}
	return value, nil
}

func buildPreview(channel uint8, stream Stream, handle uint32) ([]byte, error) {
	value := previewEnvelope{}
	value.Preview.Version = "1.1"
	value.Preview.Channel = channel
	value.Preview.Handle = handle
	value.Preview.Stream = stream
	return marshalDocument(value)
}

func buildStopPreview(channel uint8, handle uint32) ([]byte, error) {
	value := stopPreviewEnvelope{}
	value.Preview.Version = "1.1"
	value.Preview.Channel = channel
	value.Preview.Handle = handle
	return marshalDocument(value)
}
