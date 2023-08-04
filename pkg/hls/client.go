package hls

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
)

type Client struct {
	medias    []*core.Media
	receivers []*core.Receiver

	playlist string

	recv int
}

func NewClient(res *http.Response) (*Client, error) {
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`#EXT-X-STREAM-INF.+?CODECS="([^"]+).+?\n(\S+)`)
	b := re.FindStringSubmatch(string(body))
	if b == nil {
		return nil, errors.New("hls: wrong playlist: " + string(body))
	}

	ref, err := url.Parse(b[2])
	if err != nil {
		return nil, err
	}

	ref = res.Request.URL.ResolveReference(ref)

	return &Client{
		medias:   ParseCodecs(b[1]),
		playlist: ref.String(),
	}, nil
}

func (c *Client) Handle() error {
	req, err := http.NewRequest("GET", c.playlist, nil)
	if err != nil {
		return err
	}

	client := http.Client{Timeout: core.ConnDialTimeout}
	reader := mpegts.NewReader()

	var lastSegment []byte
	var lastTime time.Time

	for {
		if wait := time.Second - time.Since(lastTime); wait > 0 {
			time.Sleep(wait)
		}

		// 1. Load playlist
		res, err := client.Do(req)
		if err != nil {
			return err
		}

		playlist, err := io.ReadAll(res.Body)
		if err != nil {
			return err
		}

		lastTime = time.Now()

		//log.Printf("[hls] load playlist\n%s", playlist)

		// 2. Remove all previous segments from playlist
		if i := bytes.Index(playlist, lastSegment); i > 0 {
			playlist = playlist[i:]
		}

		for playlist != nil {
			// 3. Get link to new segment
			var segment []byte
			if segment, playlist = GetSegment(playlist); segment == nil {
				break
			}

			//log.Printf("[hls] load segment: %s", segment)

			ref, err := url.Parse(string(segment))
			if err != nil {
				return err
			}

			ref = req.URL.ResolveReference(ref)
			if res, err = client.Get(ref.String()); err != nil {
				return err
			}

			body, err := io.ReadAll(res.Body)
			if err != nil {
				return err
			}

			reader.AppendBuffer(body)

		reading:
			for {
				packet := reader.GetPacket()
				if packet == nil {
					break
				}

				for _, receiver := range c.receivers {
					if receiver.ID == packet.PayloadType {
						receiver.WriteRTP(packet)
						continue reading
					}
				}
			}

			lastSegment = segment
		}
	}
}

func ParseCodecs(codecs string) (medias []*core.Media) {
	for _, name := range strings.Split(codecs, ",") {
		var extra string
		if i := strings.IndexByte(name, '.'); i > 0 {
			name, extra = name[:i], name[i+1:]
		}

		switch name {
		case "avc1":
			codec := &core.Codec{
				Name:        core.CodecH264,
				ClockRate:   90000,
				FmtpLine:    "profile-level-id=" + extra,
				PayloadType: core.PayloadTypeRAW,
			}
			media := &core.Media{
				Kind:      core.KindVideo,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			}
			medias = append(medias, media)

		case "mp4a":
			if extra != "40.2" {
				continue
			}

			codec := &core.Codec{
				Name: core.CodecAAC,
			}
			media := &core.Media{
				Kind:      core.KindAudio,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			}
			medias = append(medias, media)
		}
	}

	return
}

func GetSegment(src []byte) (segment, left []byte) {
	for ok := false; !ok; {
		ok = bytes.HasPrefix(src, []byte("#EXTINF"))

		i := bytes.IndexByte(src, '\n') + 1
		if i == 0 {
			return nil, nil
		}

		src = src[i:]
	}

	if i := bytes.IndexByte(src, '\n'); i > 0 {
		return src[:i], src[i+1:]
	}

	return src, nil
}
