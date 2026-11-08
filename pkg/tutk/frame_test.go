package tutk

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func videoFragment(frameType byte, total, index uint16, payload []byte, frameSize uint32) []byte {
	headerSize := 28
	if frameType&8 != 0 {
		headerSize = 36
	}
	b := make([]byte, headerSize)
	b[0], b[1] = ChannelPVideo, frameType
	inner := b[headerSize-20:]
	inner[0], inner[1] = ChannelPVideo, frameType&1
	binary.LittleEndian.PutUint16(inner[4:], total)
	if frameType&1 != 0 {
		index = frameInfoSize
	}
	binary.LittleEndian.PutUint16(inner[6:], index)
	binary.LittleEndian.PutUint32(inner[16:], 1)
	b = append(b, payload...)
	if frameType&1 != 0 {
		fi := make([]byte, frameInfoSize)
		fi[0] = CodecH264
		binary.LittleEndian.PutUint32(fi[8:], 50000)
		binary.LittleEndian.PutUint32(fi[16:], frameSize)
		binary.LittleEndian.PutUint32(fi[20:], 123)
		b = append(b, fi...)
	}
	binary.LittleEndian.PutUint16(b[headerSize-12:], uint16(len(b)-headerSize))
	return b
}

func TestExtendedEndFragment(t *testing.T) {
	// HL_BC sends 0x09 at the end of multi-packet P-frames too. The 0x28
	// field is the FRAMEINFO length, not packet index 40.
	for _, frameType := range []byte{FrameTypeEndSingle, FrameTypeEndMulti, FrameTypeStartAlt, FrameTypeEndExt} {
		t.Run(fmt.Sprintf("0x%02x", frameType), func(t *testing.T) {
			h := NewFrameHandler(false)
			defer h.Close()
			h.Handle(videoFragment(FrameTypeCont, 3, 0, []byte("first"), 0))
			h.Handle(videoFragment(FrameTypeStart, 3, 1, []byte("second"), 0))
			end := videoFragment(frameType, 3, 2, []byte("last"), 15)
			hdr := ParsePacketHeader(end)
			require.Equal(t, uint16(2), hdr.PktIdx)
			require.True(t, hdr.HasFrameInfo)
			h.Handle(end)
			select {
			case pkt := <-h.Recv():
				require.Equal(t, []byte("firstsecondlast"), pkt.Payload)
				require.Equal(t, uint32(123), pkt.FrameNo)
				require.Equal(t, CodecH264, pkt.Codec)
			default:
				t.Fatal("complete frame was dropped")
			}
		})
	}
}

func TestContinuationPacketIndex40(t *testing.T) {
	for _, frameType := range []byte{FrameTypeCont, FrameTypeContAlt, FrameTypeStart} {
		hdr := ParsePacketHeader(videoFragment(frameType, 100, 40, []byte("fragment"), 0))
		require.Equal(t, uint16(40), hdr.PktIdx)
		require.False(t, hdr.HasFrameInfo)
	}
}

func TestSingleExtendedFrame(t *testing.T) {
	h := NewFrameHandler(false)
	defer h.Close()
	h.Handle(videoFragment(FrameTypeStartAlt, 1, 0, []byte("single"), 6))
	select {
	case pkt := <-h.Recv():
		require.Equal(t, []byte("single"), pkt.Payload)
	default:
		t.Fatal("single packet frame was dropped")
	}
}
