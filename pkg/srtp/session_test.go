package srtp

import (
	"math/rand"
	"testing"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

// https://datatracker.ietf.org/doc/html/rfc6716#

// Input:

// For code 0 packets, the TOC byte is immediately followed by N-1 bytes
// of compressed data for a single frame (where N is the size of the
// packet)
//
// 0                   1                   2                   3
// 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// | config  |s|0|0|                                               |
// +-+-+-+-+-+-+-+-+                                               |
// |                    Compressed frame 1 (N-1 bytes)...          :
// :                                                               |
// |                                                               |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+



var CONFIG_10 = []uint8{SILK_NB_10, SILK_MB_10, SILK_WB_10, HYBRID_SWB_10, HYBRID_FB_10, CELT_NB_10, CELT_WB_10, CELT_SWB_10, CELT_FB_10}
var CONFIG_20 = []uint8{SILK_NB_20, SILK_MB_20, SILK_WB_20, HYBRID_SWB_20, HYBRID_FB_20, CELT_NB_20, CELT_WB_20, CELT_SWB_20, CELT_FB_20}

// ffmpeg sends 20ms Opus frames, and 20ms frame is requested (Wi-Fi connection)
// only timestamp is mangled
func TestOpusPacket20(t *testing.T) {

	for _, conf := range CONFIG_20 {
		audioSession := &Session{
			AudioFrameDuration: 20,
		}
		var timestamp uint32 = 0
		for i := 0; i <= 15; i++ {
			packet := newRandomOpusPacket(conf<<3, uint16(i))
			repacketizedPacket := audioSession.repacketizeOpus(packet.Clone())
			require.Equal(t, packet.Payload, repacketizedPacket.Payload)
			require.Equal(t, packet.SequenceNumber, repacketizedPacket.SequenceNumber)
			// compare timestamp
			if i == 0 {
				// first packet, timestamp no change
				require.Equal(t, packet.Timestamp, repacketizedPacket.Timestamp)
			} else {
				require.Equal(t, timestamp+uint32(audioSession.AudioFrameDuration)*SAMPLE_RATE, repacketizedPacket.Timestamp)
			}
			timestamp = repacketizedPacket.Timestamp
		}
	}
}

// ffmpeg sends 20ms Opus frames, and 60ms frame is requested (Cellular connection)
// merge three frames into one packet
func TestOpusRepacketize20to60(t *testing.T) {
	audioSession := &Session{
		AudioFrameDuration: 60,
	}

	packets := make([]*rtp.Packet, 3)
	for _, conf := range CONFIG_20 {
		for i := 1; i <= 15; i++ {
			packet := newRandomOpusPacket(conf<<3, uint16(i))
			repacketizedPacket := audioSession.repacketizeOpus(packet.Clone())

			if i%3 != 0 {
				require.Nil(t, repacketizedPacket)
				packets[i%3] = packet
			} else {
				require.NotNil(t, repacketizedPacket)
				// compare sequence number
				if packets[0] != nil {
					require.Equal(t, packets[0].SequenceNumber+1, repacketizedPacket.SequenceNumber)
				}
				// compare timestamp
				if packets[0] != nil {
					require.Equal(t, packets[0].Timestamp+uint32(audioSession.AudioFrameDuration)*SAMPLE_RATE, repacketizedPacket.Timestamp)
				}

				// compare config bytes
				require.Equal(t, byte(conf<<3+3), repacketizedPacket.Payload[0])
				// compaer frame count byte (M)
				//  0 1 2 3 4 5 6 7
				// +-+-+-+-+-+-+-+-+
				// |v|p|     M     |
				// +-+-+-+-+-+-+-+-+
				require.Equal(t, byte(1<<7+3), repacketizedPacket.Payload[1])
				// compare M-1 frame lengths
				firstLength := int(repacketizedPacket.Payload[2])
				lengthIt := 3
				if firstLength >= 252 {
					// second byte is needed
					firstLength += int(repacketizedPacket.Payload[3]) * 4
					lengthIt++
				}
				secondLength := int(repacketizedPacket.Payload[lengthIt])
				if secondLength >= 252 {
					// second byte is needed
					secondLength += int(repacketizedPacket.Payload[lengthIt+1]) * 4
					lengthIt++
				}
				thirdLength := len(repacketizedPacket.Payload) - 1 - lengthIt - firstLength - secondLength
				require.Equal(t, len(packets[1].Payload)-1, firstLength)
				require.Equal(t, len(packets[2].Payload)-1, secondLength)
				require.Equal(t, len(packet.Payload)-1, thirdLength)
				// compare payloads
				require.Equal(t, packets[1].Payload[1:], repacketizedPacket.Payload[lengthIt+1:lengthIt+1+firstLength])
				require.Equal(t, packets[2].Payload[1:], repacketizedPacket.Payload[lengthIt+1+firstLength:lengthIt+1+firstLength+secondLength])
				require.Equal(t, packet.Payload[1:], repacketizedPacket.Payload[lengthIt+1+firstLength+secondLength:])
				packets[0] = repacketizedPacket
			}
		}
	}

}

// ffmpeg sends <20ms Opus frames, and 20ms or 60ms frame is requested
// not handled. can be implemented similarly by merging multiple frames into one packet
func TestOpusPacket10(t *testing.T) {
	frameDurations := []uint8{20, 60}
	for _, frameDuration := range frameDurations {
		audioSession := &Session{
			AudioFrameDuration: frameDuration,
		}
		for _, conf := range CONFIG_10 {
			for i := 0; i <= 15; i++ {
				packet := newRandomOpusPacket(conf<<3, uint16(i))
				// return packet as is
				repacketizedPacket := audioSession.repacketizeOpus(packet.Clone())
				require.Equal(t, packet, repacketizedPacket)
			}
		}
	}
}

// ffmpeg sends >20ms Opus frames, and 20ms or 60ms frame is requested
// only timestamp is mangled
// e.g, If ffmpeg args contains -frame_duration 60, three 20ms frames will be sent.
// After timestamp mangling, audio is working in Cellular but not in Wi-Fi connections.
func TestOpusPacket60(t *testing.T) {
	frameDurations := []uint8{20, 60}
	for _, frameDuration := range frameDurations {
		for _, conf := range CONFIG_20 {
			audioSession := &Session{
				AudioFrameDuration: frameDuration,
			}
			var timestamp uint32 = 0
			for i := 0; i <= 15; i++ {
				packet := newRandomOpusPacket(conf<<3, uint16(i))
				packet.Payload[0] |= 0b00000011 // code 3
				repacketizedPacket := audioSession.repacketizeOpus(packet.Clone())
				require.Equal(t, packet.Payload, repacketizedPacket.Payload)
				// compare timestamp
				if i == 0 {
					// first packet, timestamp no change
					require.Equal(t, packet.Timestamp, repacketizedPacket.Timestamp)
				} else {
					require.Equal(t, timestamp+uint32(audioSession.AudioFrameDuration)*SAMPLE_RATE, repacketizedPacket.Timestamp)
				}
				timestamp = repacketizedPacket.Timestamp
			}
		}
	}
}

// return a rtp packet with random payload
func newRandomOpusPacket(configByte byte, seqenceNumber uint16) *rtp.Packet {
	rawPacket := make([]byte, rand.Intn(1276)+1)
	rawPacket[0] = configByte
	for i := 1; i < len(rawPacket); i++ {
		rawPacket[i] = byte(rand.Intn(256))
	}
	return &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: seqenceNumber,
		},
		Payload: rawPacket,
	}
}
