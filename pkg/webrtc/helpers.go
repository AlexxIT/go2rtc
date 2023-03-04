package webrtc

import (
	"errors"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/ice/v2"
	"github.com/pion/stun"
	"github.com/pion/webrtc/v3"
	"hash/crc32"
	"net"
	"strconv"
	"strings"
	"time"
)

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

func MimeType(codec *streamer.Codec) string {
	switch codec.Name {
	case streamer.CodecH264:
		return webrtc.MimeTypeH264
	case streamer.CodecH265:
		return webrtc.MimeTypeH265
	case streamer.CodecVP8:
		return webrtc.MimeTypeVP8
	case streamer.CodecVP9:
		return webrtc.MimeTypeVP9
	case streamer.CodecAV1:
		return webrtc.MimeTypeAV1
	case streamer.CodecPCMU:
		return webrtc.MimeTypePCMU
	case streamer.CodecPCMA:
		return webrtc.MimeTypePCMA
	case streamer.CodecOpus:
		return webrtc.MimeTypeOpus
	case streamer.CodecG722:
		return webrtc.MimeTypeG722
	}
	panic("not implemented")
}

// 4.1.2.2.  Guidelines for Choosing Type and Local Preferences
// The RECOMMENDED values are 126 for host candidates, 100
// for server reflexive candidates, 110 for peer reflexive candidates,
// and 0 for relayed candidates.

// We use new priority 120 for Manual Host. It is lower than real Host,
// but more then any other candidates.

const PriorityManualHost = (1 << 24) * uint32(120)
const PriorityLocalUDP = (1 << 8) * uint32(65535)
const PriorityLocalTCPPassive = (1 << 8) * uint32((1<<13)*4+8191)
const PriorityComponentRTP = uint32(256 - ice.ComponentRTP)

func CandidateManualHostUDP(host string, port int) string {
	foundation := crc32.ChecksumIEEE([]byte("host" + host + "udp4"))
	priority := PriorityManualHost + PriorityLocalUDP + PriorityComponentRTP

	// 1. Foundation
	// 2. Component, always 1 because RTP
	// 3. udp or tcp
	// 4. Priority
	// 5. Host - IP4 or IP6 or domain name
	// 6. Port
	// 7. typ host
	return fmt.Sprintf(
		"candidate:%d 1 udp %d %s %d typ host",
		foundation, priority, host, port,
	)
}

func CandidateManualHostTCPPassive(address string, port int) string {
	foundation := crc32.ChecksumIEEE([]byte("host" + address + "tcp4"))
	priority := PriorityManualHost + PriorityLocalTCPPassive + PriorityComponentRTP

	return fmt.Sprintf(
		"candidate:%d 1 tcp %d %s %d typ host tcptype passive",
		foundation, priority, address, port,
	)
}
