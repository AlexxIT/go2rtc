package onvif

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

// producer is a source with fixed medias that never sends packets
type producer struct {
	medias []*core.Media
	done   chan struct{}
}

func (p *producer) GetMedias() []*core.Media { return p.medias }

func (p *producer) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return core.NewReceiver(media, codec), nil
}

func (p *producer) Start() error { <-p.done; return nil }

func (p *producer) Stop() error { close(p.done); return nil }

func TestGetProfiles(t *testing.T) {
	// a 2560x1920 H264 stream (Amcrest AD410), parameter sets in the SDP
	streams.HandleFunc("camera", func(string) (core.Producer, error) {
		return &producer{
			medias: []*core.Media{
				{Kind: core.KindVideo, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{{
					Name: core.CodecH264, ClockRate: 90000, PayloadType: 96,
					FmtpLine: "packetization-mode=1;sprop-parameter-sets=Z0IAMukAUAHjQgAAB9IAAOqcCAA=,aM48gA==",
				}}},
			},
			done: make(chan struct{}),
		}, nil
	})
	for _, name := range []string{"cam_sub", "cam_main"} {
		_, err := streams.New(name, "camera:"+name)
		require.NoError(t, err)
	}

	body := `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><GetProfiles xmlns="http://www.onvif.org/ver10/media/wsdl"/></s:Body></s:Envelope>`
	w := httptest.NewRecorder()
	onvifDeviceService(w, httptest.NewRequest("POST", "/onvif/media_service", strings.NewReader(body)))
	b := w.Body.String()

	require.Contains(t, b, "<tt:Encoding>H264</tt:Encoding>")
	require.Contains(t, b, "<tt:Resolution><tt:Width>2560</tt:Width><tt:Height>1920</tt:Height></tt:Resolution>")
	require.Contains(t, b, "<tt:H264Profile>Baseline</tt:H264Profile>")
	require.Contains(t, b, `token="cam_main"`)
	require.Contains(t, b, `token="cam_sub"`)
}
