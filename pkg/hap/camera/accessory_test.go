package camera

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/stretchr/testify/require"
)

func TestNilCharacter(t *testing.T) {
	var res SetupEndpoints
	char := &hap.Character{}
	err := char.ReadTLV8(&res)
	require.NotNil(t, err)
	require.NotNil(t, strings.Contains(err.Error(), "can't read value"))
}

type testTLV8 struct {
	name    string
	value   string
	actual  any
	expect  any
	noequal bool
}

func (test testTLV8) run(t *testing.T) {
	if test.actual == nil {
		return
	}

	src := &hap.Character{Value: test.value, Format: hap.FormatTLV8}
	err := src.ReadTLV8(test.actual)
	require.Nil(t, err)

	require.Equal(t, test.expect, test.actual)

	dst := &hap.Character{Format: hap.FormatTLV8}
	err = dst.Write(test.actual)
	require.Nil(t, err)

	a, _ := base64.StdEncoding.DecodeString(test.value)
	b, _ := base64.StdEncoding.DecodeString(dst.Value.(string))
	t.Logf("%x\n", a)
	t.Logf("%x\n", b)

	if !test.noequal {
		require.Equal(t, test.value, dst.Value)
	}
}

func TestAqaraG3(t *testing.T) {
	tests := []testTLV8{
		{
			name:   "120",
			value:  "AQEA",
			actual: &StreamingStatus{},
			expect: &StreamingStatus{
				Status: StreamingStatusAvailable,
			},
		},
		{
			name:   "114",
			value:  "AaoBAQACEQEBAQIBAAAAAgECAwEABAEAAwsBAoAHAgI4BAMBHgAAAwsBAgAFAgLQAgMBHgAAAwsBAoACAgJoAQMBHgAAAwsBAuABAgIOAQMBHgAAAwsBAkABAgK0AAMBHgAAAwsBAgAFAgLAAwMBHgAAAwsBAgAEAgIAAwMBHgAAAwsBAoACAgLgAQMBHgAAAwsBAuABAgJoAQMBHgAAAwsBAkABAgLwAAMBHg==",
			actual: &SupportedVideoStreamConfiguration{},
			expect: &SupportedVideoStreamConfiguration{
				Codecs: []VideoCodecConfiguration{
					{
						CodecType: VideoCodecTypeH264,
						CodecParams: []VideoCodecParameters{
							{
								ProfileID:  []byte{VideoCodecProfileMain},
								Level:      []byte{VideoCodecLevel31, VideoCodecLevel40},
								CVOEnabled: []byte{0},
							},
						},
						VideoAttrs: []VideoCodecAttributes{
							{Width: 1920, Height: 1080, Framerate: 30},
							{Width: 1280, Height: 720, Framerate: 30},
							{Width: 640, Height: 360, Framerate: 30},
							{Width: 480, Height: 270, Framerate: 30},
							{Width: 320, Height: 180, Framerate: 30},
							{Width: 1280, Height: 960, Framerate: 30},
							{Width: 1024, Height: 768, Framerate: 30},
							{Width: 640, Height: 480, Framerate: 30},
							{Width: 480, Height: 360, Framerate: 30},
							{Width: 320, Height: 240, Framerate: 30},
						},
					},
				},
			},
		},
		{
			name:   "115",
			value:  "AQ4BAQICCQEBAQIBAAMBAQIBAA==",
			actual: &SupportedAudioStreamConfiguration{},
			expect: &SupportedAudioStreamConfiguration{
				Codecs: []AudioCodecConfiguration{
					{
						CodecType: AudioCodecTypeAACELD,
						CodecParams: []AudioCodecParameters{
							{
								Channels:    1,
								BitrateMode: AudioCodecBitrateVariable,
								SampleRate:  []byte{AudioCodecSampleRate16Khz},
							},
						},
					},
				},
				ComfortNoiseSupport: 0,
			},
		},
		{
			name:   "116",
			value:  "AgEAAAACAQEAAAIBAg==",
			actual: &SupportedRTPConfiguration{},
			expect: &SupportedRTPConfiguration{
				SRTPCryptoType: []byte{CryptoAES_CM_128_HMAC_SHA1_80, CryptoAES_CM_256_HMAC_SHA1_80, CryptoDisabled},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestHomebridge(t *testing.T) {
	tests := []testTLV8{
		{
			name:   "114",
			value:  "AcUBAQACHQEBAAAAAQEBAAABAQICAQAAAAIBAQAAAgECAwEAAwsBAkABAgK0AAMBHgAAAwsBAkABAgLwAAMBDwAAAwsBAkABAgLwAAMBHgAAAwsBAuABAgIOAQMBHgAAAwsBAuABAgJoAQMBHgAAAwsBAoACAgJoAQMBHgAAAwsBAoACAgLgAQMBHgAAAwsBAgAFAgLQAgMBHgAAAwsBAgAFAgLAAwMBHgAAAwsBAoAHAgI4BAMBHgAAAwsBAkAGAgKwBAMBHg==",
			actual: &SupportedVideoStreamConfiguration{},
			expect: &SupportedVideoStreamConfiguration{
				Codecs: []VideoCodecConfiguration{
					{
						CodecType: VideoCodecTypeH264,
						CodecParams: []VideoCodecParameters{
							{
								ProfileID: []byte{VideoCodecProfileConstrainedBaseline, VideoCodecProfileMain, VideoCodecProfileHigh},
								Level:     []byte{VideoCodecLevel31, VideoCodecLevel32, VideoCodecLevel40},
							},
						},
						VideoAttrs: []VideoCodecAttributes{

							{Width: 320, Height: 180, Framerate: 30},
							{Width: 320, Height: 240, Framerate: 15},
							{Width: 320, Height: 240, Framerate: 30},
							{Width: 480, Height: 270, Framerate: 30},
							{Width: 480, Height: 360, Framerate: 30},
							{Width: 640, Height: 360, Framerate: 30},
							{Width: 640, Height: 480, Framerate: 30},
							{Width: 1280, Height: 720, Framerate: 30},
							{Width: 1280, Height: 960, Framerate: 30},
							{Width: 1920, Height: 1080, Framerate: 30},
							{Width: 1600, Height: 1200, Framerate: 30},
						},
					},
				},
			},
		},
		{
			name:   "116",
			value:  "AgEA",
			actual: &SupportedRTPConfiguration{},
			expect: &SupportedRTPConfiguration{
				SRTPCryptoType: []byte{CryptoAES_CM_128_HMAC_SHA1_80},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestScrypted(t *testing.T) {
	tests := []testTLV8{
		{
			name:   "114",
			value:  "AVIBAQACEwEBAQIBAAAAAgEBAAACAQIDAQADCwECAA8CAnAIAwEeAAADCwECgAcCAjgEAwEeAAADCwECAAUCAtACAwEeAAADCwECQAECAvAAAwEP",
			actual: &SupportedVideoStreamConfiguration{},
			expect: &SupportedVideoStreamConfiguration{
				Codecs: []VideoCodecConfiguration{
					{
						CodecType: VideoCodecTypeH264,
						CodecParams: []VideoCodecParameters{
							{
								ProfileID: []byte{VideoCodecProfileMain},
								Level:     []byte{VideoCodecLevel31, VideoCodecLevel32, VideoCodecLevel40},
							},
						},
						VideoAttrs: []VideoCodecAttributes{
							{Width: 3840, Height: 2160, Framerate: 30},
							{Width: 1920, Height: 1080, Framerate: 30},
							{Width: 1280, Height: 720, Framerate: 30},
							{Width: 320, Height: 240, Framerate: 15},
						},
					},
				},
			},
		},
		{
			name:   "115",
			value:  "AScBAQMCIgEBAQIBAAMBAAAAAwEAAAADAQEAAAMBAQAAAwECAAADAQICAQA=",
			actual: &SupportedAudioStreamConfiguration{},
			expect: &SupportedAudioStreamConfiguration{
				Codecs: []AudioCodecConfiguration{
					{
						CodecType: AudioCodecTypeOpus,
						CodecParams: []AudioCodecParameters{
							{
								Channels:    1,
								BitrateMode: AudioCodecBitrateVariable,
								SampleRate: []byte{
									AudioCodecSampleRate8Khz, AudioCodecSampleRate8Khz,
									AudioCodecSampleRate16Khz, AudioCodecSampleRate16Khz,
									AudioCodecSampleRate24Khz, AudioCodecSampleRate24Khz,
								},
							},
						},
					},
				},
				ComfortNoiseSupport: 0,
			},
		},
		{
			name:   "116",
			value:  "AgEAAAACAQI=",
			actual: &SupportedRTPConfiguration{},
			expect: &SupportedRTPConfiguration{
				SRTPCryptoType: []byte{CryptoAES_CM_128_HMAC_SHA1_80, CryptoDisabled},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestHass(t *testing.T) {
	tests := []testTLV8{
		{
			name:  "114",
			value: "AdABAQACFQMBAAEBAAEBAQEBAgIBAAIBAQIBAgMMAQJAAQICtAADAg8AAwwBAkABAgLwAAMCDwADDAECQAECArQAAwIeAAMMAQJAAQIC8AADAh4AAwwBAuABAgIOAQMCHgADDAEC4AECAmgBAwIeAAMMAQKAAgICaAEDAh4AAwwBAoACAgLgAQMCHgADDAECAAQCAkACAwIeAAMMAQIABAICAAMDAh4AAwwBAgAFAgLQAgMCHgADDAECAAUCAsADAwIeAAMMAQKABwICOAQDAh4A",
		},
		{
			name:  "115",
			value: "AQ4BAQMCCQEBAQIBAAMBAgEOAQEDAgkBAQECAQADAQECAQA=",
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}
