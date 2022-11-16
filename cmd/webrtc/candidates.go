package webrtc

import (
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/pion/sdp/v3"
)

var candidates []string

func AddCandidate(address string) {
	candidates = append(candidates, address)
}

func asyncCandidates(ctx *api.Context) {
	for _, address := range candidates {
		address, err := webrtc.LookupIP(address)
		if err != nil {
			log.Warn().Err(err).Caller().Send()
			continue
		}

		cand, err := webrtc.NewCandidate(address)
		if err != nil {
			log.Warn().Err(err).Caller().Send()
			continue
		}

		log.Trace().Str("candidate", cand).Msg("[webrtc] config")

		ctx.Write(&streamer.Message{Type: webrtc.MsgTypeCandidate, Value: cand})
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

		cand, err := webrtc.NewCandidate(address)
		if err != nil {
			log.Warn().Err(err).Msg("[webrtc] candidate")
			continue
		}

		md.WithPropertyAttribute(cand)
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

func candidateHandler(ctx *api.Context, msg *streamer.Message) {
	if ctx.Consumer == nil {
		return
	}
	if conn := ctx.Consumer.(*webrtc.Conn); conn != nil {
		log.Trace().Str("candidate", msg.Value.(string)).Msg("[webrtc] remote")
		conn.Push(msg)
	}
}
