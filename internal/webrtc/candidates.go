package webrtc

import (
	"net"

	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/pion/sdp/v3"
)

type Address struct {
	Host    string
	Port    string
	Network string
	Offset  int
}

func (a *Address) Marshal() string {
	host := a.Host
	if host == "stun" {
		ip, err := webrtc.GetCachedPublicIP()
		if err != nil {
			return ""
		}
		host = ip.String()
	}

	switch a.Network {
	case "udp":
		return webrtc.CandidateManualHostUDP(host, a.Port, a.Offset)
	case "tcp":
		return webrtc.CandidateManualHostTCPPassive(host, a.Port, a.Offset)
	}

	return ""
}

var addresses []*Address

func AddCandidate(address, network string) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return
	}

	offset := -1 - len(addresses) // every next candidate will have a lower priority

	switch network {
	case "tcp", "udp":
		addresses = append(addresses, &Address{host, port, network, offset})
	default:
		addresses = append(
			addresses, &Address{host, port, "udp", offset}, &Address{host, port, "tcp", offset},
		)
	}
}

func GetCandidates() (candidates []string) {
	for _, address := range addresses {
		if candidate := address.Marshal(); candidate != "" {
			candidates = append(candidates, candidate)
		}
	}
	return
}

func asyncCandidates(tr *ws.Transport, cons *webrtc.Conn) {
	tr.WithContext(func(ctx map[any]any) {
		if candidates, ok := ctx["candidate"].([]string); ok {
			// process candidates that receive before this moment
			for _, candidate := range candidates {
				_ = cons.AddCandidate(candidate)
			}

			// remove already processed candidates
			delete(ctx, "candidate")
		}

		// set variable for process candidates after this moment
		ctx["webrtc"] = cons
	})

	for _, candidate := range GetCandidates() {
		log.Trace().Str("candidate", candidate).Msg("[webrtc] config")
		tr.Write(&ws.Message{Type: "webrtc/candidate", Value: candidate})
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

	for _, candidate := range GetCandidates() {
		md.WithPropertyAttribute(candidate)
	}

	data, err := sd.Marshal()
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func candidateHandler(tr *ws.Transport, msg *ws.Message) error {
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
