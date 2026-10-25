package legacy

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/tutk"
	"github.com/pion/rtp"
)

func Dial(rawURL string) (*Producer, error) {
	client, err := NewClient(rawURL)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse(rawURL)
	query := u.Query()

	err = client.StartMedia(query.Get("subtype"), "")
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	medias, err := probe(client)
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	did := query.Get("did")
	c := &Producer{
		did: did,
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "xiaomi/legacy",
			Protocol:   "tutk+udp",
			RemoteAddr: client.RemoteAddr().String(),
			UserAgent:  client.Version(),
			Medias:     medias,
			Transport:  client,
		},
		client: client,
	}
	if len(medias) > 0 && medias[0].Kind == core.KindVideo {
		c.vcodec = medias[0].Codecs[0]
	}
	if q, ok := g2Quality(query.Get("subtype")); ok && client.model == ModelAqaraG2 {
		c.quality = q.name
	}
	if did != "" {
		active.Store(did, c)
	}
	return c, nil
}

type Producer struct {
	core.Connection
	client *Client
	did    string
	vcodec *core.Codec

	ch1     sync.Once // 0x0700 opens the reliable channel the record list comes on
	mu      sync.Mutex
	quality string        // current G2 quality name
	want    uint16        // picture height SetQuality waits for
	done    chan struct{} // closed once a keyframe of that height arrived
}

// ErrQualityUnsupported: only the Aqara G2 switches quality inside a session (verified on a Mi Home capture).
var ErrQualityUnsupported = errors.New("xiaomi: quality is only switchable on the Aqara G2")

// active: running producers by camera did, for SetQuality.
var active sync.Map

// SetQuality switches a running G2 session to fhd/hd/sd, as Mi Home does (one 0x0605 command, no
// reconnect), and returns once the camera's first keyframe of the new size has updated the codec, so
// RTSP clients that connect afterwards get an SDP that matches the picture.
func SetQuality(did, quality string) error {
	v, ok := active.Load(did)
	if !ok {
		return errors.New("xiaomi: no running session for " + did)
	}
	c := v.(*Producer)
	q, ok := g2Quality(quality)
	if !ok || c.client.model != ModelAqaraG2 {
		return ErrQualityUnsupported
	}

	c.mu.Lock()
	if c.quality == q.name {
		c.mu.Unlock()
		return nil
	}
	done := make(chan struct{})
	c.want, c.done, c.quality = q.height, done, q.name
	c.mu.Unlock()

	if err := c.client.WriteCommandJSON(0x0605, `{"channel":%d}`, q.channel); err != nil {
		return err
	}
	select {
	case <-done:
	case <-time.After(8 * time.Second):
	}
	return nil
}

// Playback moves a running G2 session to the SD card recording at unix time start (what Mi Home does when
// the timeline is dragged: one 0x0607 command (+ 0x0702 for the speed), the same video stream continues from there at 1x), or back
// to the live picture when start is 0. Like SetQuality it returns once the first keyframe of the new
// picture has updated the codec, so RTSP clients that connect afterwards get a matching SDP.
func Playback(did string, start int64, speed float64) (file string, err error) {
	v, ok := active.Load(did)
	if !ok {
		return "", errors.New("xiaomi: no running session for " + did)
	}
	c := v.(*Producer)
	if c.client.model != ModelAqaraG2 {
		return "", ErrQualityUnsupported
	}

	if start == 0 {
		// Stop, then start again: a bare 0x01ff during playback leaves the camera at 640x360 and deaf to
		// 0x0605 (verified live). Then ask for the quality this session had.
		q, _ := g2Quality(c.quality)
		done := c.expect(q.height)
		err := errors.Join(c.client.WriteCommandJSON(cmdVideoStop, `{}`), c.client.WriteCommandJSON(cmdVideoStart, `{}`), c.client.WriteCommandJSON(0x0605, `{"channel":%d}`, q.channel), c.client.WriteCommandJSON(0x0704, `{}`))
		return "", c.wait(done, err)
	}

	if err := c.client.WriteCommandJSON(0x0607, `{"keep_on":1,"start_time":"%d"}`, start); err != nil {
		return "", err
	}
	if speed < 1 {
		speed = 1
	}
	// Mi Home's speed menu (1x, 2x, 4x) is one more command; the same stream then runs that much faster.
	if err := c.client.WriteCommandJSON(0x0702, `{"speed":%v}`, speed); err != nil {
		return "", err
	}
	// 0x0609 {"result":"ok","filename":...} comes just before the recording's first frame. The camera
	// plays the nearest file it has, so the name tells what a time before the oldest recording became.
	replied := make(chan string, 1)
	go func() {
		for {
			t, data, err := c.client.ReadCommand()
			if err != nil || t == 0x0609 {
				replied <- string(data)
				return
			}
		}
	}()
	select {
	case r := <-replied:
		if m := regexp.MustCompile(`"filename":\s*"([^"]*)"`).FindStringSubmatch(r); m != nil {
			file = m[1]
		}
	case <-time.After(5 * time.Second):
		return "", errors.New("xiaomi: playback not acknowledged")
	}
	return file, c.wait(c.expect(anyHeight), nil)
}

const anyHeight = 0xFFFF

// expect arms keyframe() to close the returned channel on the next keyframe of that height (anyHeight: any).
func (c *Producer) expect(height uint16) chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.want, c.done = height, make(chan struct{})
	return c.done
}

func (c *Producer) wait(done chan struct{}, err error) error {
	if err == nil {
		select {
		case <-done:
		case <-time.After(8 * time.Second):
		}
	}
	return err
}

// Record is one SD card recording: Dur seconds starting at unix time Start.
type Record struct {
	Start int64 `json:"start"`
	Dur   int   `json:"dur"`
	Type  int   `json:"type"`
}

// Records asks the camera for its recording index over the reliable channel (see tutk/ch1.go).
func Records(did string, start, end int64) ([]Record, error) {
	v, ok := active.Load(did)
	if !ok {
		return nil, errors.New("xiaomi: no running session for " + did)
	}
	c := v.(*Producer)
	if c.client.model != ModelAqaraG2 {
		return nil, ErrQualityUnsupported
	}
	var err error
	c.ch1.Do(func() { err = c.client.WriteCommandJSON(0x0700, `{"channel":1}`) })
	if err != nil {
		return nil, err
	}
	b, err := c.client.Ch1Request(fmt.Sprintf(`{"time_start":%d,"time_end":%d}`, start, end))
	if err != nil {
		return nil, err
	}
	return parseRecords(b), nil
}

// parseRecords reads the reply of Ch1Request: an optional 8-byte (kind, size) header, then one 8-byte
// entry per recording: u32 start, u8 duration in seconds, 2 bytes unused, u8 type.
func parseRecords(b []byte) []Record {
	if len(b) >= 8 && binary.LittleEndian.Uint32(b) < 100 { // first reply carries a (kind, size) header
		b = b[8:]
	}
	recs := make([]Record, 0, len(b)/8)
	for ; len(b) >= 8; b = b[8:] {
		recs = append(recs, Record{int64(binary.LittleEndian.Uint32(b)), int(b[4]), int(b[7])})
	}
	return recs
}

// keyframe: a VPS packet of the picture size SetQuality is waiting for updates the codec in place.
func (c *Producer) keyframe(avcc []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.want == 0 || c.vcodec == nil || h265.NALUType(avcc) != h265.NALUTypeVPS {
		return
	}
	next := h265.AVCCToCodec(avcc)
	if _, sps, _ := h265.GetParameterSet(next.FmtpLine); len(sps) > 2 {
		if v := h265.DecodeSPS(sps); v != nil && (c.want == anyHeight || v.Height() == c.want) {
			c.vcodec.FmtpLine = next.FmtpLine
			c.want = 0
			close(c.done)
		}
	}
}

const codecXiaobaiPCMA = 1 // chuangmi.camera.xiaobai

func probe(client *Client) ([]*core.Media, error) {
	_ = client.SetDeadline(time.Now().Add(15 * time.Second))

	var vcodec, acodec *core.Codec
	// The camera starts every session at 640x360 and only switches to the requested quality about a
	// second later (verified on a Mi Home capture), so the first keyframe describes the wrong picture:
	// the SDP would say 360p while 1080p frames follow. Wait for the keyframe of the wanted size.
	settle := time.Now().Add(8 * time.Second)

	for {
		// 0   5000      codec
		// 2   0000      codec params
		// 4   01        active clients
		// 5   34        unknown const
		// 6   0600      unknown seq(s)
		// 8   80026801  unknown fixed
		// 12  ed8d5c69  time in sec
		// 16  4c03      time in 1/1000
		// 18  0000
		hdr, payload, err := client.ReadPacket()
		if err != nil {
			return nil, err
		}

		switch codec := hdr[0]; codec {
		case tutk.CodecH264, tutk.CodecH265:
			if vcodec == nil {
				avcc := annexb.EncodeToAVCC(payload)
				if codec == tutk.CodecH264 {
					if h264.NALUType(avcc) == h264.NALUTypeSPS {
						vcodec = h264.AVCCToCodec(avcc)
					}
				} else {
					if h265.NALUType(avcc) == h265.NALUTypeVPS {
						vcodec = h265.AVCCToCodec(avcc)
						if _, sps, _ := h265.GetParameterSet(vcodec.FmtpLine); client.wantHeight != 0 && time.Now().Before(settle) && len(sps) > 2 {
							if v := h265.DecodeSPS(sps); v != nil && v.Height() != client.wantHeight {
								vcodec = nil
							}
						}
					}
				}
			}
		case tutk.CodecPCMA, codecXiaobaiPCMA:
			if acodec == nil {
				acodec = &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}
			}
		case tutk.CodecPCML:
			if acodec == nil {
				acodec = &core.Codec{Name: core.CodecPCML, ClockRate: 8000}
			}
		case tutk.CodecAACLATM:
			if acodec == nil {
				acodec = aac.ADTSToCodec(payload)
				if acodec != nil {
					acodec.PayloadType = core.PayloadTypeRAW
				}
			}
		}

		if vcodec != nil && acodec != nil {
			break
		}
	}

	medias := []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{vcodec},
		},
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{acodec},
		},
	}
	return medias, nil
}

func (c *Producer) Protocol() string {
	return "tutk+udp"
}

func (c *Producer) Start() error {
	var audioTS uint32
	var videoSeq, audioSeq uint16

	for {
		_ = c.client.SetDeadline(time.Now().Add(5 * time.Second))
		hdr, payload, err := c.client.ReadPacket()
		if err != nil {
			return err
		}

		n := len(payload)
		c.Recv += n

		// TODO: rewrite this
		var name string
		var pkt *core.Packet

		switch codec := hdr[0]; codec {
		case tutk.CodecH264, tutk.CodecH265:
			pkt = &core.Packet{
				Header: rtp.Header{
					SequenceNumber: videoSeq,
					Timestamp:      core.Now90000(),
				},
				Payload: annexb.EncodeToAVCC(payload),
			}
			videoSeq++

			if codec == tutk.CodecH264 {
				name = core.CodecH264
			} else {
				name = core.CodecH265
				c.keyframe(pkt.Payload)
			}

		case tutk.CodecPCMA, tutk.CodecPCML, codecXiaobaiPCMA:
			pkt = &core.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					SequenceNumber: audioSeq,
					Timestamp:      audioTS,
				},
				Payload: payload,
			}
			audioSeq++

			switch codec {
			case tutk.CodecPCMA, codecXiaobaiPCMA:
				name = core.CodecPCMA
				audioTS += uint32(n)
			case tutk.CodecPCML:
				name = core.CodecPCML
				audioTS += uint32(n / 2) // because 16bit
			}

		case tutk.CodecAACLATM:
			// The camera sends ADTS frames; RTP AAC (mode=AAC-hbr) carries raw AUs. Android's
			// decoder rejects the ADTS header ("Invalid AAC stream") and Media3 never starts.
			if aac.IsADTS(payload) {
				payload = payload[aac.ADTSHeaderLen(payload):]
			}
			pkt = &core.Packet{
				Header: rtp.Header{
					SequenceNumber: audioSeq,
					Timestamp:      audioTS,
				},
				Payload: payload,
			}
			audioSeq++

			name = core.CodecAAC
			audioTS += 1024
		}

		for _, recv := range c.Receivers {
			if recv.Codec.Name == name {
				recv.WriteRTP(pkt)
				break
			}
		}
	}
}

func (c *Producer) Stop() error {
	if c.did != "" {
		active.CompareAndDelete(c.did, c)
	}
	_ = c.client.StopMedia()
	return c.Connection.Stop()
}
