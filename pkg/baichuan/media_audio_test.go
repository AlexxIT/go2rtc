package baichuan

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
)

func TestRecoverAudio(t *testing.T) {
	audio := recoveryAACFixture()
	headerOnly := append([]byte(nil), audio[:15]...)
	binary.LittleEndian.PutUint16(headerOnly[4:], 7)
	binary.LittleEndian.PutUint16(headerOnly[6:], 7)
	headerOnly[12], headerOnly[13] = 0, 7<<5
	reserved := append([]byte(nil), audio...)
	reserved[10] = reserved[10]&0xc3 | 13<<2
	for _, test := range []struct {
		name string
		data []byte
		want int
	}{
		{"confirmed", append(append([]byte{9}, audio...), recoveryPFixture("H265")...), 1},
		{"unconfirmed tail", audio, 0},
		{"junk successor", append(append([]byte(nil), audio...), bytes.Repeat([]byte{9}, 32)...), 0},
		{"ordered pair", append(append(append([]byte(nil), audio...), audio...), infoFixture()...), 2},
		{"truncated", audio[:15], 0},
		{"empty ADTS body", append(headerOnly, infoFixture()...), 0},
		{"reserved AAC rate", append(reserved, infoFixture()...), 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := append([]byte(nil), test.data...)
			var got [][]byte
			err := RecoverAudio(test.data, func(packet MediaPacket) error {
				if packet.Kind != MediaAAC {
					t.Fatalf("unexpected kind %d", packet.Kind)
				}
				got = append(got, packet.Data)
				return nil
			})
			if err != nil || len(got) != test.want || !bytes.Equal(test.data, original) {
				t.Fatalf("packets=%d want=%d err=%v input changed=%t", len(got), test.want, err, !bytes.Equal(test.data, original))
			}
			for _, data := range got {
				if !bytes.Equal(data, audio[8:]) {
					t.Fatal("audio payload changed")
				}
			}
		})
	}
	for _, field := range []int{4, 6, 8, 9, 12} {
		data := append(append([]byte(nil), audio...), infoFixture()...)
		data[field] ^= 1
		if err := RecoverAudio(data, func(MediaPacket) error { t.Fatal("accepted invalid audio"); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	want := errors.New("stop")
	if err := RecoverAudio(append(audio, infoFixture()...), func(MediaPacket) error { return want }); err != want {
		t.Fatalf("callback error lost: %v", err)
	}
	adpcm := make([]byte, 24)
	binary.LittleEndian.PutUint32(adpcm, mediaADPCM)
	binary.LittleEndian.PutUint16(adpcm[4:], 9)
	binary.LittleEndian.PutUint16(adpcm[6:], 9)
	binary.LittleEndian.PutUint16(adpcm[8:], 0x0100)
	adpcm[16] = 0x12
	for _, valid := range []bool{true, false} {
		if !valid {
			adpcm[14] = 89 // Invalid IMA step index inside an otherwise valid envelope.
		}
		count := 0
		err := RecoverAudio(append(append([]byte(nil), adpcm...), infoFixture()...), func(packet MediaPacket) error {
			count++
			if packet.Kind != MediaADPCM || len(packet.Data) != 5 {
				t.Fatalf("ADPCM packet: %+v", packet)
			}
			return nil
		})
		if err != nil || (count == 1) != valid {
			t.Fatalf("ADPCM accepted=%d valid=%t err=%v", count, valid, err)
		}
	}
}

func TestRecoverAudioShortVideoEndpoint(t *testing.T) {
	aac := recoveryAACFixture()
	aacPadded := make([]byte, 24)
	binary.LittleEndian.PutUint32(aacPadded, mediaAAC)
	binary.LittleEndian.PutUint16(aacPadded[4:6], 9)
	binary.LittleEndian.PutUint16(aacPadded[6:8], 9)
	copy(aacPadded[8:], []byte{0xff, 0xf1, 0x50, 0x80, 0x01, 0x20, 0x00, 0x55, 0x66})
	adpcm := make([]byte, 24)
	binary.LittleEndian.PutUint32(adpcm, mediaADPCM)
	binary.LittleEndian.PutUint16(adpcm[4:6], 9)
	binary.LittleEndian.PutUint16(adpcm[6:8], 9)
	binary.LittleEndian.PutUint16(adpcm[8:10], 0x0100)
	adpcm[16] = 0x12

	for _, test := range []struct {
		name string
		data []byte
		kind MediaKind
		want []byte
	}{
		{"AAC plain endpoint", aac, MediaAAC, aac[8:]},
		{"AAC padded endpoint", aacPadded, MediaAAC, aacPadded[8:17]},
		{"AAC unpadded endpoint", aacPadded[:17], MediaAAC, aacPadded[8:17]},
		{"ADPCM padded endpoint", adpcm, MediaADPCM, adpcm[12:17]},
		{"ADPCM unpadded endpoint", adpcm[:17], MediaADPCM, adpcm[12:17]},
		{"interior audio retains successor", append(append([]byte(nil), aac...), infoFixture()...), MediaAAC, aac[8:]},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := append([]byte(nil), test.data...)
			var got []MediaPacket
			err := (MediaPacket{Data: test.data, ShortVideo: true}).RecoverAudio(func(packet MediaPacket) error {
				got = append(got, packet)
				return nil
			})
			if err != nil || len(got) != 1 || got[0].Kind != test.kind || !bytes.Equal(got[0].Data, test.want) || !bytes.Equal(test.data, original) {
				t.Fatalf("packets=%+v err=%v input changed=%t", got, err, !bytes.Equal(test.data, original))
			}
		})
	}

	badPadding := append([]byte(nil), aacPadded...)
	badPadding[len(badPadding)-1] = 1
	badADTS := append([]byte(nil), aacPadded...)
	badADTS[13] ^= 0x20
	badSizes := append([]byte(nil), aacPadded...)
	binary.LittleEndian.PutUint16(badSizes[6:8], 8)
	badADPCMPadding := append([]byte(nil), adpcm...)
	badADPCMPadding[len(badADPCMPadding)-1] = 1
	badADPCMIndex := append([]byte(nil), adpcm...)
	badADPCMIndex[14] = 89
	for _, data := range [][]byte{
		badPadding,
		badADTS,
		badSizes,
		badADPCMPadding,
		badADPCMIndex,
		aacPadded[:16],
		aac[:7],
		append(append([]byte(nil), aac...), 9),
	} {
		if err := (MediaPacket{Data: data, ShortVideo: true}).RecoverAudio(func(MediaPacket) error {
			t.Fatalf("accepted invalid endpoint %x", data)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := (MediaPacket{Data: aac, ShortVideo: false}).RecoverAudio(func(MediaPacket) error {
		t.Fatal("accepted endpoint without ShortVideo corroboration")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestShortVideoEndpointDoesNotRecoverTruncatedAAC(t *testing.T) {
	data := bytes.Clone(recoveryAACFixture()[:15])
	binary.LittleEndian.PutUint16(data[4:6], 7)
	binary.LittleEndian.PutUint16(data[6:8], 7) // The ADTS header still declares an eight-byte frame.
	count := 0
	err := (MediaPacket{Data: data, ShortVideo: true}).RecoverAudio(func(MediaPacket) error {
		count++
		return nil
	})
	if err != nil || count != 0 {
		t.Fatalf("truncated AAC endpoint recovered %d packets, err=%v", count, err)
	}
}

func TestRecoverAudioFrameSizeLimit(t *testing.T) {
	frame := append(append([]byte(nil), recoveryAACFixture()...), infoFixture()...)
	data := make([]byte, defaultMaxMediaFrame)
	copy(data, frame)
	original := append([]byte(nil), data...)
	want := errors.New("stop")
	count := 0
	err := RecoverAudio(data, func(MediaPacket) error { count++; return want })
	if err != want || count != 1 || !bytes.Equal(data, original) {
		t.Fatalf("at-limit audio count=%d err=%v input-changed=%t", count, err, !bytes.Equal(data, original))
	}

	data = append(data, 0)
	count = 0
	err = RecoverAudio(data, func(MediaPacket) error { count++; return nil })
	if err == nil || err.Error() != fmt.Sprintf("baichuan: rejected video exceeds limit %d", defaultMaxMediaFrame) || count != 0 ||
		!bytes.Equal(data[:defaultMaxMediaFrame], original) || data[defaultMaxMediaFrame] != 0 {
		t.Fatalf("over-limit audio count=%d err=%v input-changed=%t", count, err,
			!bytes.Equal(data[:defaultMaxMediaFrame], original) || data[defaultMaxMediaFrame] != 0)
	}
}

func TestRecoverAudioPacketCountLimit(t *testing.T) {
	audio, successor := recoveryAACFixture(), infoFixture()
	stream := func(n int) []byte {
		data := make([]byte, 0, n*len(audio)+len(successor))
		for range n {
			data = append(data, audio...)
		}
		return append(data, successor...)
	}
	for _, test := range []struct {
		name  string
		count int
		want  int
		err   string
	}{
		{name: "exact maximum", count: maxMediaPackets, want: maxMediaPackets},
		{name: "one over", count: maxMediaPackets + 1, want: maxMediaPackets,
			err: fmt.Sprintf("baichuan: recovered audio count exceeds limit %d", maxMediaPackets)},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := stream(test.count)
			original := append([]byte(nil), data...)
			count := 0
			err := RecoverAudio(data, func(MediaPacket) error { count++; return nil })
			if count != test.want || (test.err == "" && err != nil) || (test.err != "" && (err == nil || err.Error() != test.err)) || !bytes.Equal(data, original) {
				t.Fatalf("audio count=%d want=%d err=%v input-changed=%t", count, test.want, err, !bytes.Equal(data, original))
			}
		})
	}
}
