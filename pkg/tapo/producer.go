package tapo

import (
	"encoding/json"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
)

func (c *Client) GetMedias() []*core.Media {
	if c.medias == nil {
		c.probe()

		video := &core.Codec{Name: core.CodecH264, ClockRate: 90000, PayloadType: core.PayloadTypeRAW}
		if c.videoType == mpegts.StreamTypeH265 {
			video.Name = core.CodecH265
		}

		audio := &core.Codec{Name: core.CodecPCMA, ClockRate: 8000, PayloadType: 8}
		if c.audioType == mpegts.StreamTypePrivateOPUS {
			audio = &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2, PayloadType: core.PayloadTypeRAW}
		}

		// don't know if all Tapo has this capabilities...
		c.medias = []*core.Media{
			{
				Kind:      core.KindVideo,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{video},
			},
			{
				Kind:      core.KindAudio,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{audio},
			},
			{
				Kind:      core.KindAudio,
				Direction: core.DirectionSendonly,
				Codecs: []*core.Codec{
					{Name: core.CodecPCMA, ClockRate: 8000, PayloadType: 8},
				},
			},
		}
	}

	return c.medias
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	for _, track := range c.receivers {
		if track.Codec == codec {
			return track, nil
		}
	}

	if err := c.SetupStream(); err != nil {
		return nil, err
	}

	track := core.NewReceiver(media, codec)
	switch media.Kind {
	case core.KindVideo:
		if codec.Name == core.CodecH265 {
			track.ID = mpegts.StreamTypeH265
		} else {
			track.ID = mpegts.StreamTypeH264
		}
	case core.KindAudio:
		if codec.Name == core.CodecOpus {
			track.ID = mpegts.StreamTypePrivateOPUS
		} else {
			track.ID = mpegts.StreamTypePCMATapo
		}
	}
	c.receivers = append(c.receivers, track)
	return track, nil
}

func (c *Client) Start() error {
	return c.Handle()
}

func (c *Client) Stop() error {
	for _, receiver := range c.receivers {
		receiver.Close()
	}
	c.stopBackchannel()
	return c.Close()
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &core.Connection{
		ID:         core.ID(c),
		FormatName: c.url.Scheme,
		Protocol:   "http",
		Medias:     c.medias,
		Recv:       c.recv,
		Receivers:  c.receivers,
		Send:       c.send,
	}
	c.backMu.Lock()
	if c.sender != nil {
		info.Senders = []*core.Sender{c.sender}
	}
	for _, bs := range c.backSenders {
		info.Senders = append(info.Senders, bs.sender)
	}
	c.backMu.Unlock()
	if c.conn1 != nil {
		info.RemoteAddr = c.conn1.RemoteAddr().String()
	}
	return json.Marshal(info)
}
