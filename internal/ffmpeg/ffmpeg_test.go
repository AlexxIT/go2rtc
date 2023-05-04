package ffmpeg

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestParseArgs(t *testing.T) {
	args := parseArgs("rtsp://example.com#video=h264#rotate=180")
	assert.Equal(t, "ffmpeg -hide_banner -allowed_media_types video -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_transport tcp -i rtsp://example.com -c:v libx264 -g 50 -profile:v high -level:v 4.1 -preset:v superfast -tune:v zerolatency -pix_fmt:v yuvj420p -an -vf transpose=1,transpose=1 -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}", args.String())

	args = parseArgs("rtsp://example.com#video=h264#rotate=180#hardware=vaapi")
	assert.Equal(t, "ffmpeg -hide_banner -hwaccel vaapi -hwaccel_output_format vaapi -allowed_media_types video -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_transport tcp -i rtsp://example.com -c:v h264_vaapi -g 50 -bf 0 -profile:v high -level:v 4.1 -sei:v 0 -an -vf format=vaapi|nv12,hwupload,transpose_vaapi=4 -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}", args.String())

	args = parseArgs("/media/bbb.mp4#video=mjpeg")
	assert.Equal(t, "ffmpeg -hide_banner -re -i /media/bbb.mp4 -c:v mjpeg -an -f mjpeg -", args.String())

	args = parseArgs("/media/bbb.mp4#video=mjpeg#hardware=vaapi")
	assert.Equal(t, "ffmpeg -hide_banner -hwaccel vaapi -hwaccel_output_format vaapi -re -i /media/bbb.mp4 -c:v mjpeg_vaapi -an -vf format=vaapi|nv12,hwupload -f mjpeg -", args.String())

	args = parseArgs("device?video=0&input_format=mjpeg&video_size=1920x1080")
	assert.Equal(t, `ffmpeg -hide_banner -f dshow -input_format mjpeg -video_size 1920x1080 -i video="0" -c copy -f mjpeg -`, args.String())
}
