package baichuan

import (
	"encoding/binary"
	"testing"
)

func TestDefaultTalkConfigSelectsADPCMProfile(t *testing.T) {
	t.Parallel()

	cfg, err := defaultTalkConfig(2, &TalkAbility{
		Version: "1.1",
		DuplexList: []talkDuplexOption{
			{Duplex: "FDX"},
		},
		AudioStreamModeList: []talkAudioStreamMode{
			{AudioStreamMode: "followVideoStream"},
		},
		AudioConfigList: []talkAudioConfigOption{
			{
				AudioConfig: TalkAudioConfig{
					AudioType:       "adpcm",
					SampleRate:      16000,
					SamplePrecision: 16,
					LengthPerEncode: 1016,
					SoundTrack:      "mono",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("defaultTalkConfig() error = %v", err)
	}

	if got, want := cfg.ChannelID, uint8(2); got != want {
		t.Fatalf("ChannelID = %d, want %d", got, want)
	}
	if got, want := cfg.AudioConfig.SampleRate, uint16(16000); got != want {
		t.Fatalf("SampleRate = %d, want %d", got, want)
	}
	if got, want := cfg.AudioConfig.LengthPerEncode, uint16(1016); got != want {
		t.Fatalf("LengthPerEncode = %d, want %d", got, want)
	}
}

func TestSerializeTalkADPCMBlock(t *testing.T) {
	t.Parallel()

	block := []byte{0x34, 0x12, 0x05, 0x00, 0xaa, 0xbb, 0xcc, 0xdd}
	const seq uint16 = 0x1234
	payload := serializeTalkADPCMBlock(block, seq)

	if got, want := binary.LittleEndian.Uint32(payload[0:4]), uint32(bcmediaADPCM); got != want {
		t.Fatalf("magic = %#x, want %#x", got, want)
	}
	if got, want := binary.LittleEndian.Uint16(payload[4:6]), uint16(len(block)+4); got != want {
		t.Fatalf("payload size = %d, want %d", got, want)
	}
	if got, want := binary.LittleEndian.Uint16(payload[8:10]), uint16(bcmediaADPCMHeader); got != want {
		t.Fatalf("header magic = %#x, want %#x", got, want)
	}
	if got, want := binary.LittleEndian.Uint16(payload[10:12]), seq; got != want {
		t.Fatalf("seq = %#x, want %#x", got, want)
	}
}

func TestADPCMEncoderRoundTripBlock(t *testing.T) {
	t.Parallel()

	input := []int16{
		0, 500, -500, 1000, -1000, 1500, -1500, 2000, -2000,
		2500, -2500, 3000, -3000, 3500, -3500, 4000, -4000, 4500,
	}

	encoder := &ADPCMEncoder{}
	block, err := encoder.EncodeBlock(input)
	if err != nil {
		t.Fatalf("EncodeBlock() error = %v", err)
	}

	decoder := &ADPCMDecoder{}
	decoded := decoder.Decode(block)

	if got, want := len(decoded), len(input); got != want {
		t.Fatalf("decoded sample count = %d, want %d", got, want)
	}

	var totalError int64
	for i := range input {
		diff := int64(input[i]) - int64(decoded[i])
		if diff < 0 {
			diff = -diff
		}
		totalError += diff
	}
	avgError := totalError / int64(len(input))
	if avgError > 2500 {
		t.Fatalf("average reconstruction error = %d, want <= 2500", avgError)
	}
}
