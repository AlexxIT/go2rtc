package tuya

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// The skill sampleRate field is unreliable (0 on some cameras, 90000 on
// others), and the video RTP clock rate is always 90000 regardless
// (RFC 7798 for H265, RFC 6184 for H264).
func TestGetVideoCodecs_ClockRate(t *testing.T) {
	c := &TuyaClient{skill: &Skill{Videos: []VideoSkill{
		{StreamType: 2, CodecType: 4, SampleRate: 0},     // HEVC, sampleRate unset
		{StreamType: 4, CodecType: 2, SampleRate: 90000}, // H264, sampleRate set
	}}}

	codecs := c.GetVideoCodecs()
	if len(codecs) != 2 {
		t.Fatalf("expected 2 codecs, got %d", len(codecs))
	}

	if codecs[0].Name != core.CodecH265 {
		t.Errorf("codec 0: expected H265, got %s", codecs[0].Name)
	}
	if codecs[1].Name != core.CodecH264 {
		t.Errorf("codec 1: expected H264, got %s", codecs[1].Name)
	}

	for i, codec := range codecs {
		if codec.ClockRate != 90000 {
			t.Errorf("codec %d (%s): expected ClockRate 90000, got %d", i, codec.Name, codec.ClockRate)
		}
	}
}
