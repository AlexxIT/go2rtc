package ffmpeg

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/ffmpeg"
	"github.com/stretchr/testify/require"
)

func TestParseArgsFile(t *testing.T) {
	tests := []struct {
		name   string
		source string
		expect string
	}{
		{
			name:   "[FILE] all tracks will be copied without transcoding codecs",
			source: "/media/bbb.mp4",
			expect: `ffmpeg -hide_banner -re -i /media/bbb.mp4 -c copy -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`,
		},
		{
			name:   "[FILE] video will be transcoded to H264, audio will be skipped",
			source: "/media/bbb.mp4#video=h264",
			expect: `ffmpeg -hide_banner -re -i /media/bbb.mp4 -c:v libx264 -g 50 -profile:v high -level:v 4.1 -preset:v superfast -tune:v zerolatency -pix_fmt:v yuv420p -an -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`,
		},
		{
			name:   "[FILE] video will be copied, audio will be transcoded to pcmu",
			source: "/media/bbb.mp4#video=copy#audio=pcmu",
			expect: `ffmpeg -hide_banner -re -i /media/bbb.mp4 -c:v copy -c:a pcm_mulaw -ar:a 8000 -ac:a 1 -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`,
		},
		{
			name:   "[FILE] video will be transcoded to H265 and rotate 270º, audio will be skipped",
			source: "/media/bbb.mp4#video=h265#rotate=-90",
			expect: `ffmpeg -hide_banner -re -i /media/bbb.mp4 -c:v libx265 -g 50 -profile:v main -level:v 5.1 -preset:v superfast -tune:v zerolatency -an -vf "transpose=2" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`,
		},
		{
			name:   "[FILE] video will be output for MJPEG to pipe, audio will be skipped",
			source: "/media/bbb.mp4#video=mjpeg",
			expect: `ffmpeg -hide_banner -re -i /media/bbb.mp4 -c:v mjpeg -an -f mjpeg -`,
		},
		{
			name:   "https://github.com/AlexxIT/go2rtc/issues/509",
			source: "ffmpeg:test.mp4#raw=-ss 00:00:20",
			expect: `ffmpeg -hide_banner -re -i ffmpeg:test.mp4 -ss 00:00:20 -c copy -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := parseArgs(test.source)
			require.Equal(t, test.expect, args.String())
		})
	}
}

func TestParseArgsDevice(t *testing.T) {
	// [DEVICE] video will be output for MJPEG to pipe, with size 1920x1080
	args := parseArgs("device?video=0&video_size=1920x1080")
	require.Equal(t, `ffmpeg -hide_banner -f dshow -video_size 1920x1080 -i "video=0" -c copy -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [DEVICE] video will be transcoded to H265 with framerate 20, audio will be skipped
	//args = parseArgs("device?video=0&video_size=1280x720&framerate=20#video=h265#audio=pcma")
	args = parseArgs("device?video=0&framerate=20#video=h265")
	require.Equal(t, `ffmpeg -hide_banner -f dshow -framerate 20 -i "video=0" -c:v libx265 -g 50 -profile:v main -level:v 5.1 -preset:v superfast -tune:v zerolatency -an -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	args = parseArgs("device?video=FaceTime HD Camera&audio=Microphone (High Definition Audio Device)")
	require.Equal(t, `ffmpeg -hide_banner -f dshow -i "video=FaceTime HD Camera:audio=Microphone (High Definition Audio Device)" -c copy -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())
}

func TestParseArgsIpCam(t *testing.T) {
	// [HTTP] video will be copied
	args := parseArgs("http://example.com")
	require.Equal(t, `ffmpeg -hide_banner -fflags nobuffer -flags low_delay -i http://example.com -c copy -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [HTTP-MJPEG] video will be transcoded to H264
	args = parseArgs("http://example.com#video=h264")
	require.Equal(t, `ffmpeg -hide_banner -fflags nobuffer -flags low_delay -i http://example.com -c:v libx264 -g 50 -profile:v high -level:v 4.1 -preset:v superfast -tune:v zerolatency -pix_fmt:v yuv420p -an -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [HLS] video will be copied, audio will be skipped
	args = parseArgs("https://example.com#video=copy")
	require.Equal(t, `ffmpeg -hide_banner -fflags nobuffer -flags low_delay -i https://example.com -c:v copy -an -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [RTSP] video will be copied without transcoding codecs
	args = parseArgs("rtsp://example.com")
	require.Equal(t, `ffmpeg -hide_banner -allowed_media_types video+audio -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp://example.com -c copy -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [RTSP] video with resize to 1280x720, should be transcoded, so select H265
	args = parseArgs("rtsp://example.com#video=h265#width=1280#height=720")
	require.Equal(t, `ffmpeg -hide_banner -allowed_media_types video -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp://example.com -c:v libx265 -g 50 -profile:v main -level:v 5.1 -preset:v superfast -tune:v zerolatency -an -vf "scale=1280:720" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [RTSP] video will be copied, changing RTSP transport from TCP to UDP+TCP
	args = parseArgs("rtsp://example.com#input=rtsp/udp")
	require.Equal(t, `ffmpeg -hide_banner -allowed_media_types video+audio -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -i rtsp://example.com -c copy -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [RTMP] video will be copied, changing RTSP transport from TCP to UDP+TCP
	args = parseArgs("rtmp://example.com#input=rtsp/udp")
	require.Equal(t, `ffmpeg -hide_banner -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -i rtmp://example.com -c copy -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())
}

func TestParseArgsAudio(t *testing.T) {
	// [AUDIO] audio will be transcoded to AAC, video will be skipped
	args := parseArgs("rtsp:///example.com#audio=aac")
	require.Equal(t, `ffmpeg -hide_banner -allowed_media_types audio -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp:///example.com -c:a aac -vn -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [AUDIO] audio will be transcoded to AAC/16000, video will be skipped
	args = parseArgs("rtsp:///example.com#audio=aac/16000")
	require.Equal(t, `ffmpeg -hide_banner -allowed_media_types audio -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp:///example.com -c:a aac -ar:a 16000 -ac:a 1 -vn -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [AUDIO] audio will be transcoded to OPUS, video will be skipped
	args = parseArgs("rtsp:///example.com#audio=opus")
	require.Equal(t, `ffmpeg -hide_banner -allowed_media_types audio -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp:///example.com -c:a libopus -application:a lowdelay -min_comp 0 -vn -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [AUDIO] audio will be transcoded to PCMU, video will be skipped
	args = parseArgs("rtsp:///example.com#audio=pcmu")
	require.Equal(t, `ffmpeg -hide_banner -allowed_media_types audio -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp:///example.com -c:a pcm_mulaw -ar:a 8000 -ac:a 1 -vn -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [AUDIO] audio will be transcoded to PCMU/16000, video will be skipped
	args = parseArgs("rtsp:///example.com#audio=pcmu/16000")
	require.Equal(t, `ffmpeg -hide_banner -allowed_media_types audio -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp:///example.com -c:a pcm_mulaw -ar:a 16000 -ac:a 1 -vn -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [AUDIO] audio will be transcoded to PCMU/48000, video will be skipped
	args = parseArgs("rtsp:///example.com#audio=pcmu/48000")
	require.Equal(t, `ffmpeg -hide_banner -allowed_media_types audio -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp:///example.com -c:a pcm_mulaw -ar:a 48000 -ac:a 1 -vn -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [AUDIO] audio will be transcoded to PCMA, video will be skipped
	args = parseArgs("rtsp:///example.com#audio=pcma")
	require.Equal(t, `ffmpeg -hide_banner -allowed_media_types audio -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp:///example.com -c:a pcm_alaw -ar:a 8000 -ac:a 1 -vn -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [AUDIO] audio will be transcoded to PCMA/16000, video will be skipped
	args = parseArgs("rtsp:///example.com#audio=pcma/16000")
	require.Equal(t, `ffmpeg -hide_banner -allowed_media_types audio -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp:///example.com -c:a pcm_alaw -ar:a 16000 -ac:a 1 -vn -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [AUDIO] audio will be transcoded to PCMA/48000, video will be skipped
	args = parseArgs("rtsp:///example.com#audio=pcma/48000")
	require.Equal(t, `ffmpeg -hide_banner -allowed_media_types audio -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp:///example.com -c:a pcm_alaw -ar:a 48000 -ac:a 1 -vn -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())
}

func TestParseArgsHwVaapi(t *testing.T) {
	tests := []struct {
		name   string
		source string
		expect string
	}{
		{
			name:   "[HTTP-MJPEG] video will be transcoded to H264",
			source: "http:///example.com#video=h264#hardware=vaapi",
			expect: `ffmpeg -hide_banner -hwaccel vaapi -hwaccel_output_format vaapi -hwaccel_flags allow_profile_mismatch -fflags nobuffer -flags low_delay -i http:///example.com -c:v h264_vaapi -g 50 -bf 0 -profile:v high -level:v 4.1 -sei:v 0 -an -vf "format=vaapi|nv12,hwupload,scale_vaapi=out_color_matrix=bt709:out_range=tv:format=nv12" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`,
		},
		{
			name:   "[RTSP] video with rotation, should be transcoded, so select H264",
			source: "rtsp://example.com#video=h264#rotate=180#hardware=vaapi",
			expect: `ffmpeg -hide_banner -hwaccel vaapi -hwaccel_output_format vaapi -hwaccel_flags allow_profile_mismatch -allowed_media_types video -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp://example.com -c:v h264_vaapi -g 50 -bf 0 -profile:v high -level:v 4.1 -sei:v 0 -an -vf "format=vaapi|nv12,hwupload,transpose_vaapi=4,scale_vaapi=out_color_matrix=bt709:out_range=tv:format=nv12" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`,
		},
		{
			name:   "[RTSP] video with resize to 1280x720, should be transcoded, so select H265",
			source: "rtsp://example.com#video=h265#width=1280#height=720#hardware=vaapi",
			expect: `ffmpeg -hide_banner -hwaccel vaapi -hwaccel_output_format vaapi -hwaccel_flags allow_profile_mismatch -allowed_media_types video -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp://example.com -c:v hevc_vaapi -g 50 -bf 0 -profile:v main -level:v 5.1 -sei:v 0 -an -vf "format=vaapi|nv12,hwupload,scale_vaapi=1280:720" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`,
		},
		{
			name:   "[FILE] video will be output for MJPEG to pipe, audio will be skipped",
			source: "/media/bbb.mp4#video=mjpeg#hardware=vaapi",
			expect: `ffmpeg -hide_banner -hwaccel vaapi -hwaccel_output_format vaapi -hwaccel_flags allow_profile_mismatch -re -i /media/bbb.mp4 -c:v mjpeg_vaapi -an -vf "format=vaapi|nv12,hwupload" -f mjpeg -`,
		},
		{
			name:   "[DEVICE] MJPEG video with size 1920x1080 will be transcoded to H265",
			source: "device?video=0&video_size=1920x1080#video=h265#hardware=vaapi",
			expect: `ffmpeg -hide_banner -hwaccel vaapi -hwaccel_output_format vaapi -hwaccel_flags allow_profile_mismatch -f dshow -video_size 1920x1080 -i "video=0" -c:v hevc_vaapi -g 50 -bf 0 -profile:v main -level:v 5.1 -sei:v 0 -an -vf "format=vaapi|nv12,hwupload" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := parseArgs(test.source)
			require.Equal(t, test.expect, args.String())
		})
	}
}

func _TestParseArgsHwV4l2m2m(t *testing.T) {
	// [HTTP-MJPEG] video will be transcoded to H264
	args := parseArgs("http:///example.com#video=h264#hardware=v4l2m2m")
	require.Equal(t, `ffmpeg -hide_banner -fflags nobuffer -flags low_delay -i http:///example.com -c:v h264_v4l2m2m -g 50 -bf 0 -an -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [RTSP] video with rotation, should be transcoded, so select H264
	args = parseArgs("rtsp://example.com#video=h264#rotate=180#hardware=v4l2m2m")
	require.Equal(t, `ffmpeg -hide_banner -allowed_media_types video -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp://example.com -c:v h264_v4l2m2m -g 50 -bf 0 -an -vf "transpose=1,transpose=1" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [RTSP] video with resize to 1280x720, should be transcoded, so select H265
	args = parseArgs("rtsp://example.com#video=h265#width=1280#height=720#hardware=v4l2m2m")
	require.Equal(t, `ffmpeg -hide_banner -allowed_media_types video -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp://example.com -c:v hevc_v4l2m2m -g 50 -bf 0 -an -vf "scale=1280:720" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [DEVICE] MJPEG video with size 1920x1080 will be transcoded to H265
	args = parseArgs("device?video=0&video_size=1920x1080#video=h265#hardware=v4l2m2m")
	require.Equal(t, `ffmpeg -hide_banner -f dshow -video_size 1920x1080 -i video="0" -c:v hevc_v4l2m2m -g 50 -bf 0 -an -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())
}

func TestParseArgsHwRKMPP(t *testing.T) {
	// [HTTP-MJPEG] video will be transcoded to H264
	args := parseArgs("http://example.com#video=h264#hardware=rkmpp")
	require.Equal(t, `ffmpeg -hide_banner -fflags nobuffer -flags low_delay -i http://example.com -c:v h264_rkmpp_encoder -g 50 -bf 0 -profile:v high -level:v 4.1 -an -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	args = parseArgs("http://example.com#video=h264#rotate=180#hardware=rkmpp")
	require.Equal(t, `ffmpeg -hide_banner -fflags nobuffer -flags low_delay -i http://example.com -c:v h264_rkmpp_encoder -g 50 -bf 0 -profile:v high -level:v 4.1 -an -vf "transpose=1,transpose=1" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	args = parseArgs("http://example.com#video=h264#height=320#hardware=rkmpp")
	require.Equal(t, `ffmpeg -hide_banner -fflags nobuffer -flags low_delay -i http://example.com -c:v h264_rkmpp_encoder -g 50 -bf 0 -profile:v high -level:v 4.1 -height 320 -an -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())
}

func _TestParseArgsHwCuda(t *testing.T) {
	// [HTTP-MJPEG] video will be transcoded to H264
	args := parseArgs("http:///example.com#video=h264#hardware=cuda")
	require.Equal(t, `ffmpeg -hide_banner -hwaccel cuda -hwaccel_output_format cuda -fflags nobuffer -flags low_delay -i http:///example.com -c:v h264_nvenc -g 50 -bf 0 -profile:v high -level:v auto -preset:v p2 -tune:v ll -an -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [RTSP] video with rotation, should be transcoded, so select H264
	args = parseArgs("rtsp://example.com#video=h264#rotate=180#hardware=cuda")
	require.Equal(t, `ffmpeg -hide_banner -hwaccel cuda -hwaccel_output_format nv12 -allowed_media_types video -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp://example.com -c:v h264_nvenc -g 50 -bf 0 -profile:v high -level:v auto -preset:v p2 -tune:v ll -an -vf "transpose=1,transpose=1,hwupload" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [RTSP] video with resize to 1280x720, should be transcoded, so select H265
	args = parseArgs("rtsp://example.com#video=h265#width=1280#height=720#hardware=cuda")
	require.Equal(t, `ffmpeg -hide_banner -hwaccel cuda -hwaccel_output_format cuda -allowed_media_types video -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp://example.com -c:v hevc_nvenc -g 50 -bf 0 -profile:v high -level:v auto -an -vf "scale_cuda=1280:720" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [DEVICE] MJPEG video with size 1920x1080 will be transcoded to H265
	args = parseArgs("device?video=0&video_size=1920x1080#video=h265#hardware=cuda")
	require.Equal(t, `ffmpeg -hide_banner -hwaccel cuda -hwaccel_output_format cuda -f dshow -video_size 1920x1080 -i video="0" -c:v hevc_nvenc -g 50 -bf 0 -profile:v high -level:v auto -an -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())
}

func _TestParseArgsHwDxva2(t *testing.T) {
	// [HTTP-MJPEG] video will be transcoded to H264
	args := parseArgs("http:///example.com#video=h264#hardware=dxva2")
	require.Equal(t, `ffmpeg -hide_banner -hwaccel dxva2 -hwaccel_output_format dxva2_vld -fflags nobuffer -flags low_delay -i http:///example.com -c:v h264_qsv -g 50 -bf 0 -profile:v high -level:v 4.1 -async_depth:v 1 -an -vf "hwmap=derive_device=qsv,format=qsv" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [RTSP] video with rotation, should be transcoded, so select H264
	args = parseArgs("rtsp://example.com#video=h264#rotate=180#hardware=dxva2")
	require.Equal(t, `ffmpeg -hide_banner -hwaccel dxva2 -hwaccel_output_format dxva2_vld -allowed_media_types video -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp://example.com -c:v h264_qsv -g 50 -bf 0 -profile:v high -level:v 4.1 -async_depth:v 1 -an -vf "hwmap=derive_device=qsv,format=qsv,transpose=1,transpose=1" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [RTSP] video with resize to 1280x720, should be transcoded, so select H265
	args = parseArgs("rtsp://example.com#video=h265#width=1280#height=720#hardware=dxva2")
	require.Equal(t, `ffmpeg -hide_banner -hwaccel dxva2 -hwaccel_output_format dxva2_vld -allowed_media_types video -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp://example.com -c:v hevc_qsv -g 50 -bf 0 -profile:v high -level:v 5.1 -async_depth:v 1 -an -vf "hwmap=derive_device=qsv,format=qsv,scale_qsv=1280:720" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [FILE] video will be output for MJPEG to pipe, audio will be skipped
	args = parseArgs("/media/bbb.mp4#video=mjpeg#hardware=dxva2")
	require.Equal(t, `ffmpeg -hide_banner -hwaccel dxva2 -hwaccel_output_format dxva2_vld -re -i /media/bbb.mp4 -c:v mjpeg_qsv -profile:v high -level:v 5.1 -an -vf "hwmap=derive_device=qsv,format=qsv" -f mjpeg -`, args.String())

	// [DEVICE] MJPEG video with size 1920x1080 will be transcoded to H265
	args = parseArgs("device?video=0&video_size=1920x1080#video=h265#hardware=dxva2")
	require.Equal(t, `ffmpeg -hide_banner -hwaccel dxva2 -hwaccel_output_format dxva2_vld -f dshow -video_size 1920x1080 -i video="0" -c:v hevc_qsv -g 50 -bf 0 -profile:v high -level:v 5.1 -async_depth:v 1 -an -vf "hwmap=derive_device=qsv,format=qsv" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())
}

func _TestParseArgsHwVideotoolbox(t *testing.T) {
	// [HTTP-MJPEG] video will be transcoded to H264
	args := parseArgs("http:///example.com#video=h264#hardware=videotoolbox")
	require.Equal(t, `ffmpeg -hide_banner -hwaccel videotoolbox -hwaccel_output_format videotoolbox_vld -fflags nobuffer -flags low_delay -i http:///example.com -c:v h264_videotoolbox -g 50 -bf 0 -profile:v high -level:v 4.1 -an -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [RTSP] video with rotation, should be transcoded, so select H264
	args = parseArgs("rtsp://example.com#video=h264#rotate=180#hardware=videotoolbox")
	require.Equal(t, `ffmpeg -hide_banner -hwaccel videotoolbox -hwaccel_output_format videotoolbox_vld -allowed_media_types video -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp://example.com -c:v h264_videotoolbox -g 50 -bf 0 -profile:v high -level:v 4.1 -an -vf "transpose=1,transpose=1" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [RTSP] video with resize to 1280x720, should be transcoded, so select H265
	args = parseArgs("rtsp://example.com#video=h265#width=1280#height=720#hardware=videotoolbox")
	require.Equal(t, `ffmpeg -hide_banner -hwaccel videotoolbox -hwaccel_output_format videotoolbox_vld -allowed_media_types video -fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i rtsp://example.com -c:v hevc_videotoolbox -g 50 -bf 0 -profile:v high -level:v 5.1 -an -vf "scale=1280:720" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())

	// [DEVICE] MJPEG video with size 1920x1080 will be transcoded to H265
	args = parseArgs("device?video=0&video_size=1920x1080#video=h265#hardware=videotoolbox")
	require.Equal(t, `ffmpeg -hide_banner -hwaccel videotoolbox -hwaccel_output_format videotoolbox_vld -f dshow -video_size 1920x1080 -i video="0" -c:v hevc_videotoolbox -g 50 -bf 0 -profile:v high -level:v 5.1 -an -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())
}

func TestDeckLink(t *testing.T) {
	args := parseArgs(`DeckLink SDI (2)#video=h264#hardware=vaapi#input=-format_code Hp29 -f decklink -i "{input}"`)
	require.Equal(t, `ffmpeg -hide_banner -hwaccel vaapi -hwaccel_output_format vaapi -hwaccel_flags allow_profile_mismatch -format_code Hp29 -f decklink -i "DeckLink SDI (2)" -c:v h264_vaapi -g 50 -bf 0 -profile:v high -level:v 4.1 -sei:v 0 -an -vf "format=vaapi|nv12,hwupload,scale_vaapi=out_color_matrix=bt709:out_range=tv:format=nv12" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`, args.String())
}

func TestDrawText(t *testing.T) {
	tests := []struct {
		name   string
		source string
		expect string
	}{
		{
			source: "http:///example.com#video=h264#drawtext=fontsize=12",
			expect: `ffmpeg -hide_banner -fflags nobuffer -flags low_delay -i http:///example.com -c:v libx264 -g 50 -profile:v high -level:v 4.1 -preset:v superfast -tune:v zerolatency -pix_fmt:v yuv420p -an -vf "drawtext=fontsize=12:text='%{localtime\:%Y-%m-%d %X}'" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`,
		},
		{
			source: "http:///example.com#video=h264#width=640#drawtext=fontsize=12",
			expect: `ffmpeg -hide_banner -fflags nobuffer -flags low_delay -i http:///example.com -c:v libx264 -g 50 -profile:v high -level:v 4.1 -preset:v superfast -tune:v zerolatency -pix_fmt:v yuv420p -an -vf "scale=640:-1,drawtext=fontsize=12:text='%{localtime\:%Y-%m-%d %X}'" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`,
		},
		{
			source: "http:///example.com#video=h264#width=640#drawtext=fontsize=12#hardware=vaapi",
			expect: `ffmpeg -hide_banner -hwaccel vaapi -hwaccel_output_format nv12 -hwaccel_flags allow_profile_mismatch -fflags nobuffer -flags low_delay -i http:///example.com -c:v h264_vaapi -g 50 -bf 0 -profile:v high -level:v 4.1 -sei:v 0 -an -vf "scale=640:-1,drawtext=fontsize=12:text='%{localtime\:%Y-%m-%d %X}',hwupload" -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := parseArgs(test.source)
			require.Equal(t, test.expect, args.String())
		})
	}
}

func TestVersion(t *testing.T) {
	verAV = ffmpeg.Version61
	tests := []struct {
		name   string
		source string
		expect string
	}{
		{
			source: "/media/bbb.mp4",
			expect: `ffmpeg -hide_banner -readrate_initial_burst 0.001 -re -i /media/bbb.mp4 -c copy -user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := parseArgs(test.source)
			require.Equal(t, test.expect, args.String())
		})
	}
}
