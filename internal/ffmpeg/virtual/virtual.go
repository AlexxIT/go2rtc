package virtual

import (
	"net/url"
)

func GetInput(src string) (string, error) {
	query, err := url.ParseQuery(src)
	if err != nil {
		return "", err
	}

	// set defaults (using Add instead of Set)
	query.Add("source", "testsrc")
	query.Add("size", "1920x1080")
	query.Add("decimals", "2")

	// https://ffmpeg.org/ffmpeg-filters.html
	source := query.Get("source")
	input := "-re -f lavfi -i " + source

	sep := "=" // first separator
	for key, values := range query {
		value := values[0]

		// https://ffmpeg.org/ffmpeg-utils.html#video-size-syntax
		switch key {
		case "color", "rate", "duration", "sar":
		case "size":
			switch value {
			case "720":
				value = "1280x720"
			case "1080":
				value = "1920x1080"
			case "2K":
				value = "2560x1440"
			case "4K":
				value = "3840x2160"
			case "8K":
				value = "7680x4230" // https://reolink.com/blog/8k-resolution/
			}
		case "decimals":
			if source != "testsrc" {
				continue
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

	return input, nil
}
