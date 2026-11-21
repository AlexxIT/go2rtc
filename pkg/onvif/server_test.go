package onvif

import (
	"encoding/xml"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

func TestNewProfile(t *testing.T) {
	tests := []struct {
		name   string
		codecs []*core.Codec
		video  Video
	}{
		{
			name:  "unknown",
			video: Video{Encoding: "H264", Profile: "Main", Width: 1920, Height: 1080, FrameRate: 30, Bitrate: 8192},
		},
		{
			name: "x264",
			codecs: []*core.Codec{
				{Name: core.CodecH264, FmtpLine: "packetization-mode=1; sprop-parameter-sets=Z2QAKay0A8ARPyzcBAQFAAADAAEAAAMAPA8YMqA=,aO8Pyw==; profile-level-id=640029"},
			},
			video: Video{Encoding: "H264", Profile: "High", Width: 1920, Height: 1080, FrameRate: 30, Bitrate: 8192},
		},
		{
			name: "H265",
			codecs: []*core.Codec{
				{Name: core.CodecH265, FmtpLine: "sprop-sps=QgEBIUAAAAMAkAAAAwAAAwCWoAUCAWlnpbkShc1AQIC4QAAAAwBAAAAFFEn/eEAOpgAV+V8IBBA="},
			},
			video: Video{Encoding: "H265", Profile: "Main", Width: 640, Height: 360, FrameRate: 30, Bitrate: 8192},
		},
		{
			name:   "H265 without parameter sets",
			codecs: []*core.Codec{{Name: core.CodecH265}},
			video:  Video{Encoding: "H265", Profile: "Main", Width: 1920, Height: 1080, FrameRate: 30, Bitrate: 8192},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := NewProfile("main", test.codecs)
			require.Equal(t, "main", p.Token)
			require.Equal(t, test.video, p.Video)
		})
	}
}

func TestProfileResponses(t *testing.T) {
	main := &Profile{
		Token: "main",
		Video: Video{Encoding: "H265", Profile: "Main", Width: 2560, Height: 1440, FrameRate: 25, Bitrate: 8192},
	}
	sub := &Profile{
		Token: "sub",
		Video: Video{Encoding: "H264", Profile: "High", Width: 960, Height: 480, FrameRate: 30, Bitrate: 8192},
	}

	type config struct {
		Token     string `xml:"token,attr"`
		Encoding  string
		Width     int `xml:"Resolution>Width"`
		Height    int `xml:"Resolution>Height"`
		FrameRate int `xml:"RateControl>FrameRateLimit"`
	}
	var profiles struct {
		Profiles []struct {
			Token                     string `xml:"token,attr"`
			VideoSourceConfiguration  config `xml:"VideoSourceConfiguration"`
			VideoEncoderConfiguration config `xml:"VideoEncoderConfiguration"`
		} `xml:"Body>GetProfilesResponse>Profiles"`
	}
	b := GetProfilesResponse([]*Profile{main, sub})
	require.NoError(t, xml.Unmarshal(b, &profiles))
	require.Len(t, profiles.Profiles, 2)

	p := profiles.Profiles[0]
	require.Equal(t, "main", p.Token)
	require.Equal(t, config{Token: "main", Encoding: "H265", Width: 2560, Height: 1440, FrameRate: 25}, p.VideoEncoderConfiguration)

	p = profiles.Profiles[1]
	require.Equal(t, config{Token: "sub", Encoding: "H264", Width: 960, Height: 480, FrameRate: 30}, p.VideoEncoderConfiguration)

	// options offer exactly the current configuration; H.265 in Extension for Media1
	var options struct {
		H264 *struct {
			Width  int `xml:"ResolutionsAvailable>Width"`
			MinFPS int `xml:"FrameRateRange>Min"`
			MaxFPS int `xml:"FrameRateRange>Max"`
		} `xml:"Body>GetVideoEncoderConfigurationOptionsResponse>Options>H264"`
		H265 *struct {
			Width  int `xml:"ResolutionsAvailable>Width"`
			MinFPS int `xml:"FrameRateRange>Min"`
			MaxFPS int `xml:"FrameRateRange>Max"`
		} `xml:"Body>GetVideoEncoderConfigurationOptionsResponse>Options>Extension>H265"`
	}
	require.NoError(t, xml.Unmarshal(GetVideoEncoderConfigurationOptionsResponse(main), &options))
	require.Nil(t, options.H264)
	require.NotNil(t, options.H265)
	require.Equal(t, 2560, options.H265.Width)
	require.Equal(t, 25, options.H265.MinFPS)
	require.Equal(t, 25, options.H265.MaxFPS)

	// every other builder produces well-formed XML
	for _, b := range [][]byte{
		GetProfileResponse(main),
		GetVideoSourcesResponse([]*Profile{main, sub}),
		GetVideoSourceConfigurationsResponse([]*Profile{main, sub}),
		GetVideoSourceConfigurationResponse(sub),
		GetVideoEncoderConfigurationsResponse([]*Profile{main, sub}),
		GetVideoEncoderConfigurationResponse(sub),
		GetVideoEncoderConfigurationOptionsResponse(sub),
	} {
		require.NoError(t, xml.Unmarshal(b, new(any)))
	}
}
