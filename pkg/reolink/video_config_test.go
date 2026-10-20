package reolink

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
)

func TestVideoCachedParametersRequireStableKeyframe(t *testing.T) {
	for _, codec := range []string{"H264", "H265"} {
		base := testVideoKeyframe(codec, 0)
		key := base.Data[len(base.Data)-6:]
		partial := append(append([]byte(nil), base.Data[:6]...), key...)
		if codec == "H264" {
			partial = append(append([]byte(nil), base.Data[:8]...), key...)
		}
		if validVideoConfig(codec, annexb.EncodeToAVCC(key)) {
			t.Fatalf("%s startup accepted missing parameter sets", codec)
		}
		for _, test := range []struct {
			name      string
			data      []byte
			resync    bool
			candidate string
			want      bool
		}{
			{name: "stable", data: key, want: true},
			{name: "resync", data: key, resync: true},
			{name: "candidate", data: key, resync: true, candidate: "pending"},
			{name: "partial", data: partial},
			{name: "non-keyframe", data: testVideoPFrame(codec, 0).Data},
			{name: "parameters-only", data: base.Data[:len(base.Data)-6]},
		} {
			t.Run(codec+"/"+test.name, func(t *testing.T) {
				pipeline := &mediaPipeline{}
				if err := pipeline.configureVideo(base); err != nil {
					t.Fatal(err)
				}
				config := pipeline.videoMedia.Codecs[0].FmtpLine
				pipeline.videoResync, pipeline.videoCandidate = test.resync, test.candidate
				frame, err := pipeline.videoFrame(baichuan.MediaPacket{
					Kind: baichuan.MediaVideoI, Codec: codec, Timestamp: 1_000_000,
					Data: append([]byte(nil), test.data...),
				})
				if err != nil || frame.valid() != test.want {
					t.Fatalf("keyframe accepted=%t, want %t: %v", frame.valid(), test.want, err)
				}
				if !test.want && (!pipeline.videoResync || pipeline.videoTS.set) {
					t.Fatal("rejected keyframe released recovery or advanced the clock")
				}
				if pipeline.videoMedia.Codecs[0].FmtpLine != config {
					t.Fatal("keyframe changed the published codec")
				}
			})
		}
	}
}

func TestVideoParameterIdentityIgnoresAnnexBPadding(t *testing.T) {
	for _, codec := range []string{"H264", "H265"} {
		for _, zeros := range []int{1, 3} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/zeros%d/reverse%t", codec, zeros, reverse), func(t *testing.T) {
					base := testVideoKeyframe(codec, 0)
					marker := []byte{0, 0, 0, 1}
					padded := bytes.ReplaceAll(base.Data, marker, append(make([]byte, zeros), marker...))
					initial, next := base.Data, padded
					if reverse {
						initial, next = next, initial
					}
					pipeline := &mediaPipeline{}
					if err := pipeline.configureVideo(baichuan.MediaPacket{Codec: codec, Data: initial}); err != nil {
						t.Fatal(err)
					}
					config := pipeline.videoMedia.Codecs[0].FmtpLine
					for i := range 2 {
						frame, err := pipeline.videoFrame(baichuan.MediaPacket{
							Kind: baichuan.MediaVideoI, Codec: codec, Timestamp: uint32(i * 50_000),
							Data: append([]byte(nil), next...),
						})
						if err != nil || !frame.valid() {
							t.Fatalf("padding changed parameter identity: frame=%v err=%v", frame, err)
						}
						if !bytes.Equal(frame.packet.Payload, annexb.EncodeToAVCC(next)) {
							t.Fatal("identity normalization changed emitted NAL payloads")
						}
					}
					if pipeline.videoMedia.Codecs[0].FmtpLine != config {
						t.Fatal("padding changed the published codec")
					}
				})
			}
		}
	}
}
