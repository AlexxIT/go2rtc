package homekit

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/pion/rtp"
)

// Clip is a CMAF-ready init segment plus media fragments for one or two tracks
type Clip struct {
	Init       []byte
	Fragments  [][]byte // each is moof+mdat
	VideoCodec *core.Codec
	AudioCodec *core.Codec
}

// BuildClip packages a packet slice into CMAF init + media fragments
// When contentKey is non-nil (16 bytes AES-128), sample payloads are encrypted
// with AES-CTR and a random 16-byte IV is prepended to each fragment body
func BuildClip(packets []Packet, contentKey []byte) (*Clip, error) {
	if len(packets) == 0 {
		return nil, errString("homekit: empty clip")
	}

	var videoCodec, audioCodec *core.Codec
	for _, p := range packets {
		if p.Track == 0 && videoCodec == nil {
			videoCodec = p.Codec
		}
		if p.Track == 1 && audioCodec == nil {
			audioCodec = p.Codec
		}
	}
	if videoCodec == nil {
		return nil, errString("homekit: clip has no video")
	}

	muxer := &mp4.Muxer{}
	// track 0 = video, track 1 = audio (optional)
	muxer.AddTrack(videoCodec)
	hasAudio := audioCodec != nil
	if hasAudio {
		muxer.AddTrack(audioCodec)
	}

	init, err := muxer.GetInit()
	if err != nil {
		return nil, err
	}

	clip := &Clip{
		Init:       init,
		VideoCodec: videoCodec,
		AudioCodec: audioCodec,
	}

	for _, p := range packets {
		trackID := p.Track
		if trackID == 1 && !hasAudio {
			continue
		}
		payload := p.Payload
		if len(contentKey) >= 16 {
			enc, err := encryptAESCTR(contentKey[:16], payload)
			if err != nil {
				return nil, err
			}
			payload = enc
		}
		pkt := &rtp.Packet{
			Header: rtp.Header{
				Timestamp: p.RTPTime,
			},
			Payload: payload,
		}
		frag := muxer.GetPayload(trackID, pkt)
		clip.Fragments = append(clip.Fragments, frag)
	}

	return clip, nil
}

func encryptAESCTR(key, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	iv := make([]byte, aes.BlockSize)
	if _, err = io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}
	out := make([]byte, len(iv)+len(plain))
	copy(out, iv)
	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(out[len(iv):], plain)
	return out, nil
}
