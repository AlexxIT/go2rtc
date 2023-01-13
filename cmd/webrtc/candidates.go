package webrtc

import (
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/pion/sdp/v3"
)

var candidates []string
var networks = []string{"udp", "tcp"}

func AddCandidate(address string) {
	candidates = append(candidates, address)
}

func asyncCandidates(tr *api.Transport) {
	for _, address := range candidates {
		address, err := webrtc.LookupIP(address)
		if err != nil {
			log.Warn().Err(err).Caller().Send()
			continue
		}

		for _, network := range networks {
			cand, err := webrtc.NewCandidate(network, address)
			if err != nil {
				log.Warn().Err(err).Caller().Send()
				continue
			}

			log.Trace().Str("candidate", cand).Msg("[webrtc] config")

			tr.Write(&api.Message{Type: "webrtc/candidate", Value: cand})
		}
	}
}

func syncCanditates(answer string) (string, error) {
	if len(candidates) == 0 {
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

	for _, address := range candidates {
		var err error
		address, err = webrtc.LookupIP(address)
		if err != nil {
			log.Warn().Err(err).Msg("[webrtc] candidate")
			continue
		}

		for _, network := range networks {
			cand, err := webrtc.NewCandidate(network, address)
			if err != nil {
				log.Warn().Err(err).Msg("[webrtc] candidate")
				continue
			}

			md.WithPropertyAttribute(cand)
		}
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
	if tr.Consumer == nil {
		return nil
	}
	if conn := tr.Consumer.(*webrtc.Conn); conn != nil {
		s := msg.Value.(string)
		log.Trace().Str("candidate", s).Msg("[webrtc] remote")
		conn.AddCandidate(s)
	}
	return nil
}
