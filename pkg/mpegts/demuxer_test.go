package mpegts

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDemuxerReadsLatePMT(t *testing.T) {
	pat := testPATPacket()
	firstPMT := testPMTPacket(testStream{pid: 0x100, streamType: StreamTypeH264})
	d := NewDemuxer()

	pkt, err := d.ReadPacket(bytes.NewReader(append(pat, firstPMT...)))
	require.NoError(t, err)
	require.Equal(t, []byte{StreamTypeH264}, pkt.Payload)

	latePMT := testPMTPacket(
		testStream{pid: 0x100, streamType: StreamTypeH264},
		testStream{pid: 0x101, streamType: StreamTypeH264},
		testStream{pid: 0x91, streamType: StreamTypePCMUTapo},
	)
	_, err = d.ReadPacket(bytes.NewReader(latePMT))
	require.ErrorIs(t, err, io.EOF)
	require.Contains(t, d.pes, uint16(0x91))
	require.Contains(t, d.pes, uint16(0x101))
	require.Equal(t, byte(StreamTypeH264), d.pes[0x101].StreamType)
	require.Equal(t, byte(StreamTypePCMUTapo), d.pes[0x91].StreamType)

	audioPES := []byte{0, 0, 1, 0xC0, 0, 0, 0x80, 0, 0, 0x11, 0x22}
	audio := testTSPacket(0x91, true, audioPES)
	pkt, err = d.ReadPacket(bytes.NewReader(append(audio, audio...)))
	require.NoError(t, err)
	require.Equal(t, byte(StreamTypePCMUTapo), pkt.PayloadType)
	require.Equal(t, []byte{0x11, 0x22}, pkt.Payload[:2])
}

func TestDemuxerReadsEveryPATProgram(t *testing.T) {
	d := NewDemuxer()
	pat := testPATPacket(0x1000, 0x1001)
	firstPMT := testPMTPacketAt(0x1000, testStream{pid: 0x100, streamType: StreamTypeH264})

	pkt, err := d.ReadPacket(bytes.NewReader(append(pat, firstPMT...)))
	require.NoError(t, err)
	require.Equal(t, []byte{StreamTypeH264}, pkt.Payload)

	secondPMT := testPMTPacketAt(0x1001, testStream{pid: 0x101, streamType: StreamTypeAAC})
	_, err = d.ReadPacket(bytes.NewReader(secondPMT))
	require.ErrorIs(t, err, io.EOF)
	require.Contains(t, d.pes, uint16(0x101))
	require.Equal(t, byte(StreamTypeAAC), d.pes[0x101].StreamType)
}

func TestDemuxerMalformedPSINoPanic(t *testing.T) {
	validPAT := testPATPacket()
	validPMT := testPMTPacket(testStream{pid: 0x100, streamType: StreamTypeH264})

	badPATPointer := testTSPacket(0, true, []byte{0xFF})
	badPATLength := testPSIPacket(0, []byte{0x00, 0xBF, 0xFF})
	badPMTPointer := testTSPacket(0x1000, true, []byte{0xFF})
	badPMTLength := testPSIPacket(0x1000, []byte{0x02, 0xBF, 0xFF})
	badProgramInfo := testPMTPacket(testStream{pid: 0x100, streamType: StreamTypeH264})
	badProgramInfo[4+1+10], badProgramInfo[4+1+11] = 0xFF, 0xFF
	badESInfo := testPMTPacket(testStream{pid: 0x100, streamType: StreamTypeH264})
	badESInfo[4+1+15], badESInfo[4+1+16] = 0xFF, 0xFF
	continuation := testTSPacket(0x1000, false, []byte{0x02, 0xFF, 0xFF})

	tests := []struct {
		name   string
		packet []byte
	}{
		{"PAT pointer past packet", badPATPointer},
		{"PAT section past packet", badPATLength},
		{"PMT pointer past packet", badPMTPointer},
		{"PMT section past packet", badPMTLength},
		{"PMT program info past section", badProgramInfo},
		{"PMT ES info past section", badESInfo},
		{"PMT continuation", continuation},
	}

	for _, tt := range tests {
		t.Run(tt.name+" before first PMT", func(t *testing.T) {
			input := append(append([]byte{}, validPAT...), tt.packet...)
			_, err := NewDemuxer().ReadPacket(bytes.NewReader(input))
			require.ErrorIs(t, err, io.EOF)
		})

		t.Run(tt.name+" after first PMT", func(t *testing.T) {
			d := NewDemuxer()
			_, err := d.ReadPacket(bytes.NewReader(append(append([]byte{}, validPAT...), validPMT...)))
			require.NoError(t, err)

			_, err = d.ReadPacket(bytes.NewReader(tt.packet))
			require.ErrorIs(t, err, io.EOF)
		})
	}
}

type testStream struct {
	pid        uint16
	streamType byte
}

func testPATPacket(pmtPIDs ...uint16) []byte {
	if len(pmtPIDs) == 0 {
		pmtPIDs = []uint16{0x1000}
	}
	sectionLength := 9 + len(pmtPIDs)*4
	section := []byte{
		0x00, 0xB0 | byte(sectionLength>>8), byte(sectionLength), // table ID, section length
		0x00, 0x01, 0xC1, 0x00, 0x00, // transport stream and section header
	}
	for i, pid := range pmtPIDs {
		section = append(section, 0x00, byte(i+1), 0xE0|byte(pid>>8), byte(pid))
	}
	section = append(section, 0x00, 0x00, 0x00, 0x00) // CRC
	return testPSIPacket(0, section)
}

func testPMTPacket(streams ...testStream) []byte {
	return testPMTPacketAt(0x1000, streams...)
}

func testPMTPacketAt(pid uint16, streams ...testStream) []byte {
	sectionLength := 13 + len(streams)*5
	section := []byte{
		0x02, 0xB0 | byte(sectionLength>>8), byte(sectionLength),
		0x00, 0x01, 0xC1, 0x00, 0x00, // program and section header
		0xE1, 0x00, 0xF0, 0x00, // PCR PID and no program descriptors
	}
	for _, stream := range streams {
		section = append(section, stream.streamType, 0xE0|byte(stream.pid>>8), byte(stream.pid), 0xF0, 0x00)
	}
	section = append(section, 0x00, 0x00, 0x00, 0x00) // CRC
	return testPSIPacket(pid, section)
}

func testPSIPacket(pid uint16, section []byte) []byte {
	return testTSPacket(pid, true, append([]byte{0}, section...))
}

func testTSPacket(pid uint16, start bool, payload []byte) []byte {
	packet := bytes.Repeat([]byte{0xFF}, PacketSize)
	packet[0] = SyncByte
	packet[1] = byte(pid >> 8 & 0x1F)
	if start {
		packet[1] |= 0x40
	}
	packet[2] = byte(pid)
	packet[3] = 0x10 // payload only
	copy(packet[4:], payload)
	return packet
}
