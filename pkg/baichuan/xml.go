package baichuan

import (
	"encoding/xml"
	"fmt"
	"strings"
)

type xmlBody struct {
	XMLName    xml.Name       `xml:"body"`
	Encryption *xmlEncryption `xml:"Encryption,omitempty"`
}

type xmlEncryption struct {
	Version string `xml:"version,attr,omitempty"`
	Type    string `xml:"type"`
	Nonce   string `xml:"nonce"`
}

type xmlLoginBody struct {
	XMLName   xml.Name     `xml:"body"`
	LoginUser xmlLoginUser `xml:"LoginUser"`
	LoginNet  xmlLoginNet  `xml:"LoginNet"`
}

type xmlLoginUser struct {
	Version  string `xml:"version,attr"`
	UserName string `xml:"userName"`
	Password string `xml:"password"`
	UserVer  int    `xml:"userVer"`
}

type xmlLoginNet struct {
	Version string `xml:"version,attr"`
	Type    string `xml:"type"`
	UDPPort int    `xml:"udpPort"`
}

type xmlPreviewBody struct {
	XMLName xml.Name   `xml:"body"`
	Preview xmlPreview `xml:"Preview"`
}

type xmlPreview struct {
	Version    string `xml:"version,attr"`
	ChannelID  uint8  `xml:"channelId"`
	Handle     uint32 `xml:"handle"`
	StreamType Stream `xml:"streamType"`
}

type xmlPreviewStopBody struct {
	XMLName xml.Name       `xml:"body"`
	Preview xmlPreviewStop `xml:"Preview"`
}

type xmlPreviewStop struct {
	Version   string `xml:"version,attr"`
	ChannelID uint8  `xml:"channelId"`
	Handle    uint32 `xml:"handle"`
}

type xmlEncryptLenBody struct {
	XMLName    xml.Name `xml:"body"`
	EncryptLen int      `xml:"encryptLen"`
}

func marshalXMLDocument(v any) ([]byte, error) {
	body, err := xml.Marshal(v)
	if err != nil {
		return nil, err
	}

	return append([]byte(xml.Header), body...), nil
}

func buildLoginXML(userHash string, passwordHash string) ([]byte, error) {
	return marshalXMLDocument(xmlLoginBody{
		LoginUser: xmlLoginUser{
			Version:  "1.1",
			UserName: userHash,
			Password: passwordHash,
			UserVer:  1,
		},
		LoginNet: xmlLoginNet{
			Version: "1.1",
			Type:    "LAN",
			UDPPort: 0,
		},
	})
}

func buildPreviewXML(channel uint8, handle uint32, stream Stream) ([]byte, error) {
	return marshalXMLDocument(xmlPreviewBody{
		Preview: xmlPreview{
			Version:    "1.1",
			ChannelID:  channel,
			Handle:     handle,
			StreamType: stream,
		},
	})
}

func buildStopPreviewXML(channel uint8, handle uint32) ([]byte, error) {
	return marshalXMLDocument(xmlPreviewStopBody{
		Preview: xmlPreviewStop{
			Version:   "1.1",
			ChannelID: channel,
			Handle:    handle,
		},
	})
}

func parseNonce(xmlText string) (string, error) {
	var body xmlBody
	if err := xml.Unmarshal([]byte(xmlText), &body); err != nil {
		return "", err
	}
	if body.Encryption == nil || body.Encryption.Nonce == "" {
		return "", fmt.Errorf("nonce missing from login response")
	}
	return body.Encryption.Nonce, nil
}

func parseExtension(buf []byte) (*Extension, error) {
	if len(buf) == 0 {
		return nil, nil
	}

	var ext Extension
	if err := xml.Unmarshal(buf, &ext); err != nil {
		return nil, err
	}
	return &ext, nil
}

func trimXML(buf []byte) string {
	return strings.TrimSpace(string(buf))
}

func parseEncryptLen(xmlText string) (int, bool) {
	if xmlText == "" {
		return 0, false
	}

	var body xmlEncryptLenBody
	if err := xml.Unmarshal([]byte(xmlText), &body); err != nil {
		return 0, false
	}
	if body.EncryptLen <= 0 {
		return 0, false
	}
	return body.EncryptLen, true
}
