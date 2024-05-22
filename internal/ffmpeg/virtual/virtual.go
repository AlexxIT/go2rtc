package virtual

import (
	"net/url"
)

func GetInput(src string) string {
	query, err := url.ParseQuery(src)
	if err != nil {
		return ""
	}

	input := "-re"

	for _, video := range query["video"] {
		// https://ffmpeg.org/ffmpeg-filters.html
		sep := "=" // first separator

		if video == "" {
			video = "testsrc=decimals=2" // default video
			sep = ":"
		}

		input += " -f lavfi -i " + video

		// set defaults (using Add instead of Set)
		query.Add("size", "1920x1080")

		for key, values := range query {
			value := values[0]

			// https://ffmpeg.org/ffmpeg-utils.html#video-size-syntax
			switch key {
			case "color", "rate", "duration", "sar", "decimals":
			case "size":
				switch value {
				case "720":
					value = "1280x720" // crf=1 -> 12 Mbps
				case "1080":
					value = "1920x1080" // crf=1 -> 25 Mbps
				case "2K":
					value = "2560x1440" // crf=1 -> 43 Mbps
				case "4K":
					value = "3840x2160" // crf=1 -> 103 Mbps
				case "8K":
					value = "7680x4230" // https://reolink.com/blog/8k-resolution/
				}
			default:
				continue
			}

			input += sep + key + "=" + value
			sep = ":" // next separator
		}

		if s := query.Get("format"); s != "" {
			input += ",format=" + s
		}
	}

	return input
}

func GetInputTTS(src string) string {
	query, err := url.ParseQuery(src)
	if err != nil {
		return ""
	}

	input := `-re -f lavfi -i "flite=text='` + query.Get("text") + `'`

	// ffmpeg -f lavfi -i flite=list_voices=1
	// awb, kal, kal16, rms, slt
	if voice := query.Get("voice"); voice != "" {
		input += ":voice" + voice
	}

	return input + `"`
}
