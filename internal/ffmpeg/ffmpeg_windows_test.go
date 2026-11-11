//go:build windows

package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Device input is OS specific (dshow on Windows), see internal/ffmpeg/device

func TestParseArgsDevice(t *testing.T) {
	tests := []struct {
		name   string
		source string
		expect string
	}{
		{
			name:   "[DEVICE] video will be output for MJPEG to pipe, with size 1920x1080",
			source: "device?video=0&video_size=1920x1080",
			expect: `ffmpeg -hide_banner -f dshow -video_size 1920x1080 -i "video=0" -c copy -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`,
		},
		{
			name:   "[DEVICE] video will be transcoded to H265 with framerate 20, audio will be skipped",
			source: "device?video=0&framerate=20#video=h265",
			expect: `ffmpeg -hide_banner -f dshow -framerate 20 -i "video=0" -c:v libx265 -g 50 -profile:v main -x265-params level=5.1:high-tier=0 -preset:v superfast -tune:v zerolatency -pix_fmt:v yuv420p -an -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`,
		},
		{
			name:   "[DEVICE] video/audio",
			source: "device?video=FaceTime HD Camera&audio=Microphone (High Definition Audio Device)",
			expect: `ffmpeg -hide_banner -f dshow -i "video=FaceTime HD Camera:audio=Microphone (High Definition Audio Device)" -c copy -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := parseArgs(test.source)
			require.Equal(t, test.expect, args.String())
		})
	}
}

func TestParseArgsHwVaapiDevice(t *testing.T) {
	args := parseArgs("device?video=0&video_size=1920x1080#video=h265#hardware=vaapi")
	require.Equal(t, `ffmpeg -hide_banner -hwaccel vaapi -hwaccel_output_format vaapi -hwaccel_flags allow_profile_mismatch -f dshow -video_size 1920x1080 -i "video=0" -c:v hevc_vaapi -g 50 -bf 0 -profile:v main -level:v 5.1 -sei:v 0 -an -vf "format=vaapi|nv12,hwupload" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())
}
