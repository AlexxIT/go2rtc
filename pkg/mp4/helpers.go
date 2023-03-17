package mp4

import "github.com/AlexxIT/go2rtc/pkg/core"

// ParseQuery - like usual parse, but with mp4 param handler
func ParseQuery(query map[string][]string) []*core.Media {
	if query["mp4"] != nil {
		cons := Consumer{}
		return cons.GetMedias()
	}

	return core.ParseQuery(query)
}

const (
	waitNone byte = iota
	waitKeyframe
	waitInit
)
