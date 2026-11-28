package baichuan

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// RecoverAudio walks only an already rejected video body. Audio needs a complete
// protocol header/body and a corroborating successor inside that same body.
func RecoverAudio(data []byte, yield func(MediaPacket) error) error {
	return recoverAudio(data, false, yield)
}

// RecoverAudio uses a confirmed short-video endpoint as recovery corroboration.
func (packet MediaPacket) RecoverAudio(yield func(MediaPacket) error) error {
	return recoverAudio(packet.Data, packet.ShortVideo, yield)
}

func recoverAudio(data []byte, shortVideoEndpoint bool, yield func(MediaPacket) error) error {
	if len(data) > defaultMaxMediaFrame {
		return fmt.Errorf("baichuan: rejected video exceeds limit %d", defaultMaxMediaFrame)
	}
	p := MediaParser{limits: Limits{MaxMediaFrame: defaultMaxMediaFrame}}
	count := 0
	for len(data) >= 8 {
		// All supported audio magics end in "wb"; revisit this search when adding a kind.
		i := bytes.Index(data[2:], []byte{'w', 'b'})
		if i < 0 || len(data)-i < 8 {
			break
		}
		data = data[i:]
		magic := binary.LittleEndian.Uint32(data)
		kind := MediaAAC
		if magic == mediaADPCM {
			kind = MediaADPCM
		} else if magic != mediaAAC && magic != mediaAACV2 {
			data = data[1:]
			continue
		}

		confirmed := p.confirmRecoveryAnchor(data) == 0
		if confirmed || shortVideoEndpoint && confirmRecoveryBoundary(data, p.limits.MaxMediaFrame) == 0 {
			packet, size, complete, err := p.parseAudio(data, kind)
			if err == nil && !complete && shortVideoEndpoint && !confirmed &&
				len(data) == 8+int(binary.LittleEndian.Uint16(data[4:6])) {
				start := 8
				if kind == MediaADPCM {
					start += 4
				}
				packet = MediaPacket{Kind: kind, Data: data[start:len(data):len(data)]}
				size, complete = len(data), true
			}
			if !confirmed && size != len(data) {
				complete = false
			}
			if err == nil && complete && kind == MediaAAC {
				header := 7
				if packet.Data[1]&1 == 0 {
					header = 9
				}
				complete = len(packet.Data) > header && packet.Data[2]>>2&15 < 13
			}
			if err == nil && complete && kind == MediaADPCM {
				_, err = DecodeADPCMBlock(packet.Data)
			}
			if err == nil && complete {
				if count == maxMediaPackets {
					return fmt.Errorf("baichuan: recovered audio count exceeds limit %d", maxMediaPackets)
				}
				count++
				if err = yield(packet); err != nil {
					return err
				}
				data = data[size:]
				continue
			}
		}
		data = data[1:]
	}
	return nil
}
