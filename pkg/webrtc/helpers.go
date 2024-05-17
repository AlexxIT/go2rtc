package webrtc

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/ice/v2"
	"github.com/pion/sdp/v3"
	"github.com/pion/stun"
	"github.com/pion/webrtc/v3"
)

func UnmarshalMedias(descriptions []*sdp.MediaDescription) (medias []*core.Media) {
	// 1. Sort medias, so video will always be before audio
	// 2. Ignore application media from Hass default lovelace card
	// 3. Ignore media without direction (inactive media)
	// 4. Inverse media direction (because it is remote peer medias list)
	for _, kind := range []string{core.KindVideo, core.KindAudio} {
		for _, md := range descriptions {
			if md.MediaName.Media != kind {
				continue
			}

			media := core.UnmarshalMedia(md)
			switch media.Direction {
			case core.DirectionSendRecv:
				media.Direction = core.DirectionRecvonly
				medias = append(medias, media)

				media = media.Clone()
				media.Direction = core.DirectionSendonly

			case core.DirectionRecvonly:
				media.Direction = core.DirectionSendonly

			case core.DirectionSendonly:
				media.Direction = core.DirectionRecvonly

			case "":
				continue
			}

			// skip non-media codecs to avoid confusing users in info and logs
			media.Codecs = SkipNonMediaCodecs(media.Codecs)

			medias = append(medias, media)
		}
	}

	return
}

func SkipNonMediaCodecs(input []*core.Codec) (output []*core.Codec) {
	for _, codec := range input {
		switch codec.Name {
		case "RTX", "RED", "ULPFEC", "FLEXFEC-03":
			continue
		case "CN", "TELEPHONE-EVENT":
			continue // https://datatracker.ietf.org/doc/html/rfc7874
		}
		// VP8, VP9, H264, H265, AV1
		// OPUS, G722, PCMU, PCMA
		output = append(output, codec)
	}
	return
}

// WithResampling - will add for consumer: PCMA/0, PCMU/0, PCM/0, PCML/0
// so it can add resampling for PCMA/PCMU and repack for PCM/PCML
func WithResampling(medias []*core.Media) []*core.Media {
	for _, media := range medias {
		if media.Kind != core.KindAudio || media.Direction != core.DirectionSendonly {
			continue
		}

		var pcma, pcmu, pcm, pcml *core.Codec

		for _, codec := range media.Codecs {
			switch codec.Name {
			case core.CodecPCMA:
				if codec.ClockRate != 0 {
					pcma = codec
				} else {
					pcma = nil
				}
			case core.CodecPCMU:
				if codec.ClockRate != 0 {
					pcmu = codec
				} else {
					pcmu = nil
				}
			case core.CodecPCM:
				pcm = codec
			case core.CodecPCML:
				pcml = codec
			}
		}

		if pcma != nil {
			pcma = pcma.Clone()
			pcma.ClockRate = 0 // reset clock rate so will match any
			media.Codecs = append(media.Codecs, pcma)
		}
		if pcmu != nil {
			pcmu = pcmu.Clone()
			pcmu.ClockRate = 0
			media.Codecs = append(media.Codecs, pcmu)
		}
		if pcma != nil && pcm == nil {
			pcm = pcma.Clone()
			pcm.Name = core.CodecPCM
			media.Codecs = append(media.Codecs, pcm)
		}
		if pcma != nil && pcml == nil {
			pcml = pcma.Clone()
			pcml.Name = core.CodecPCML
			media.Codecs = append(media.Codecs, pcml)
		}
	}

	return medias
}

func NewCandidate(network, address string) (string, error) {
	i := strings.LastIndexByte(address, ':')
	if i < 0 {
		return "", errors.New("wrong candidate: " + address)
	}
	host, port := address[:i], address[i+1:]

	i, err := strconv.Atoi(port)
	if err != nil {
		return "", err
	}

	config := &ice.CandidateHostConfig{
		Network:   network,
		Address:   host,
		Port:      i,
		Component: ice.ComponentRTP,
	}

	if network == "tcp" {
		config.TCPType = ice.TCPTypePassive
	}

	cand, err := ice.NewCandidateHost(config)
	if err != nil {
		return "", err
	}

	return "candidate:" + cand.Marshal(), nil
}

func LookupIP(address string) (string, error) {
	if strings.HasPrefix(address, "stun:") {
		ip, err := GetCachedPublicIP()
		if err != nil {
			return "", err
		}
		return ip.String() + address[4:], nil
	}

	if IsIP(address) {
		return address, nil
	}

	i := strings.IndexByte(address, ':')
	ips, err := net.LookupIP(address[:i])
	if err != nil {
		return "", err
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("can't resolve: %s", address)
	}

	return ips[0].String() + address[i:], nil
}

// GetPublicIP example from https://github.com/pion/stun
func GetPublicIP() (net.IP, error) {
	conn, err := net.Dial("udp", "stun.l.google.com:19302")
	if err != nil {
		return nil, err
	}

	c, err := stun.NewClient(conn)
	if err != nil {
		return nil, err
	}

	if err = conn.SetDeadline(time.Now().Add(time.Second * 3)); err != nil {
		return nil, err
	}

	var res stun.Event

	message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if err = c.Do(message, func(e stun.Event) { res = e }); err != nil {
		return nil, err
	}
	if err = c.Close(); err != nil {
		return nil, err
	}

	if res.Error != nil {
		return nil, res.Error
	}

	var xorAddr stun.XORMappedAddress
	if err = xorAddr.GetFrom(res.Message); err != nil {
		return nil, err
	}

	return xorAddr.IP, nil
}

var cachedIP net.IP
var cachedTS time.Time

func GetCachedPublicIP() (net.IP, error) {
	now := time.Now()
	if now.After(cachedTS) {
		newIP, err := GetPublicIP()
		if err == nil {
			cachedIP = newIP
			cachedTS = now.Add(time.Minute * 5)
		} else if cachedIP == nil {
			return nil, err
		}
	}

	return cachedIP, nil
}

func IsIP(host string) bool {
	for _, i := range host {
		if i >= 'A' {
			return false
		}
	}
	return true
}

func MimeType(codec *core.Codec) string {
	switch codec.Name {
	case core.CodecH264:
		return webrtc.MimeTypeH264
	case core.CodecH265:
		return webrtc.MimeTypeH265
	case core.CodecVP8:
		return webrtc.MimeTypeVP8
	case core.CodecVP9:
		return webrtc.MimeTypeVP9
	case core.CodecAV1:
		return webrtc.MimeTypeAV1
	case core.CodecPCMU:
		return webrtc.MimeTypePCMU
	case core.CodecPCMA:
		return webrtc.MimeTypePCMA
	case core.CodecOpus:
		return webrtc.MimeTypeOpus
	case core.CodecG722:
		return webrtc.MimeTypeG722
	}
	panic("not implemented")
}

func CandidateICE(network, host, port string, priority uint32) string {
	// 1. Foundation
	// 2. Component, always 1 because RTP
	// 3. "udp" or "tcp"
	// 4. Priority
	// 5. Host - IP4 or IP6 or domain name
	// 6. Port
	// 7. "typ host"
	foundation := crc32.ChecksumIEEE([]byte("host" + host + network + "4"))
	s := fmt.Sprintf("candidate:%d 1 %s %d %s %s typ host", foundation, network, priority, host, port)
	if network == "tcp" {
		return s + " tcptype passive"
	}
	return s
}

// Priority = type << 24 + local << 8 + component
// https://www.rfc-editor.org/rfc/rfc8445#section-5.1.2.1

const PriorityHostUDP uint32 = 0x001F_FFFF |
	126<<24 | // udp host
	7<<21 // udp
const PriorityHostTCPPassive uint32 = 0x001F_FFFF |
	99<<24 | // tcp host
	4<<21 // tcp passive

// CandidateHostPriority (lower indexes has a higher priority)
func CandidateHostPriority(network string, index int) uint32 {
	switch network {
	case "udp":
		return PriorityHostUDP - uint32(index)
	case "tcp":
		return PriorityHostTCPPassive - uint32(index)
	}
	return 0
}

func UnmarshalICEServers(b []byte) ([]webrtc.ICEServer, error) {
	type ICEServer struct {
		URLs       any    `json:"urls"`
		Username   string `json:"username,omitempty"`
		Credential string `json:"credential,omitempty"`
	}

	var src []ICEServer
	if err := json.Unmarshal(b, &src); err != nil {
		return nil, err
	}

	var dst []webrtc.ICEServer
	for i := range src {
		srv := webrtc.ICEServer{
			Username:   src[i].Username,
			Credential: src[i].Credential,
		}

		switch v := src[i].URLs.(type) {
		case []string:
			srv.URLs = v
		case string:
			srv.URLs = []string{v}
		}

		dst = append(dst, srv)
	}

	return dst, nil
}
