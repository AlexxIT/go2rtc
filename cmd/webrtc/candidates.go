package webrtc

import (
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/pion/sdp/v3"
	"strconv"
	"strings"
)

type Address struct {
	Host string
	Port int
}

var addresses []Address

func AddCandidate(address string) {
	var port int

	// try to get port from address string
	if i := strings.LastIndexByte(address, ':'); i > 0 {
		if v, _ := strconv.Atoi(address[i+1:]); v != 0 {
			address = address[:i]
			port = v
		}
	}

	// use default WebRTC port
	if port == 0 {
		port, _ = strconv.Atoi(Port)
	}

	addresses = append(addresses, Address{Host: address, Port: port})
}

func GetCandidates() (candidates []string) {
	for _, address := range addresses {
		// using stun server for receive public IP-address
		if address.Host == "stun" {
			ip, err := webrtc.GetCachedPublicIP()
			if err != nil {
				continue
			}
			// this is a copy, original host unchanged
			address.Host = ip.String()
		}

		candidates = append(
			candidates,
			webrtc.CandidateHostUDP(address.Host, address.Port),
			webrtc.CandidateHostTCPPassive(address.Host, address.Port),
		)
	}

	return
}

func asyncCandidates(tr *api.Transport, cons *webrtc.Conn) {
	tr.WithContext(func(ctx map[any]any) {
		if candidates, ok := ctx["candidate"].([]string); ok {
			// process candidates that receive before this moment
			for _, candidate := range candidates {
				cons.AddCandidate(candidate)
			}

			// remove already processed candidates
			delete(ctx, "candidate")
		}

		// set variable for process candidates after this moment
		ctx["webrtc"] = cons
	})

	for _, candidate := range GetCandidates() {
		log.Trace().Str("candidate", candidate).Msg("[webrtc] config")
		tr.Write(&api.Message{Type: "webrtc/candidate", Value: candidate})
	}
}

func syncCanditates(answer string) (string, error) {
	if len(addresses) == 0 {
		return answer, nil
	}

	sd := &sdp.SessionDescription{}
	if err := sd.Unmarshal([]byte(answer)); err != nil {
		return "", err
	}

	md := sd.MediaDescriptions[0]

	_, end := md.Attribute("end-of-candidates")
	if end {
		md.Attributes = md.Attributes[:len(md.Attributes)-1]
	}

	for _, candidate := range GetCandidates() {
		md.WithPropertyAttribute(candidate)
	}

	if end {
		md.WithPropertyAttribute("end-of-candidates")
	}

	data, err := sd.Marshal()
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func candidateHandler(tr *api.Transport, msg *api.Message) error {
	// process incoming candidate in sync function
	tr.WithContext(func(ctx map[any]any) {
		candidate := msg.String()
		log.Trace().Str("candidate", candidate).Msg("[webrtc] remote")

		if cons, ok := ctx["webrtc"].(*webrtc.Conn); ok {
			// if webrtc.Server already initialized - process candidate
			_ = cons.AddCandidate(candidate)
		} else {
			// or collect candidate and process it later
			list, _ := ctx["candidate"].([]string)
			ctx["candidate"] = append(list, candidate)
		}
	})

	return nil
}
