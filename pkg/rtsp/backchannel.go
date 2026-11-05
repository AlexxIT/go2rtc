package rtsp

import (
	"errors"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
)

// addBackchannel - send backchannel with a separate RTSP session, so the running
// session (video and audio from the camera) is not interrupted by a reconnect.
// Used when two-way audio is enabled for an already playing stream.
func (c *Conn) addBackchannel(media *core.Media, codec *core.Codec, track *core.Receiver) (err error) {
	if c.backchannel != nil {
		return errors.New("rtsp: backchannel session already exists")
	}

	bc := NewClient(c.uri)
	bc.Backchannel = true
	bc.PacketSize = c.PacketSize
	bc.Timeout = c.Timeout
	bc.Transport = c.Transport
	bc.UserAgent = c.UserAgent

	if err = bc.Dial(); err != nil {
		return
	}

	defer func() {
		if err != nil {
			_ = bc.Close()
		}
	}()

	if err = bc.Describe(); err != nil {
		return
	}

	// don't retry SETUP without backchannel (Dahua fix in SetupMedia),
	// this session is only useful with backchannel, fallback is the main session reconnect
	bc.Backchannel = false

	var bcMedia *core.Media
	for _, m := range bc.Medias {
		if m.ID == media.ID && m.Direction == media.Direction {
			bcMedia = m
			break
		}
	}
	if bcMedia == nil {
		return errors.New("rtsp: backchannel media not found")
	}

	channel, err := bc.SetupMedia(bcMedia)
	if err != nil {
		return
	}

	// wait for PLAY response, so we know the camera accepts the session
	if _, err = bc.Do(&tcp.Request{Method: MethodPlay, URL: bc.URL}); err != nil {
		return
	}

	bc.playOK = true
	bc.state = StatePlay

	sender := core.NewSender(media, codec)
	sender.Handler = bc.packetWriter(track.Codec, channel, codec.PayloadType)

	if track.Codec.Name == core.CodecPCMA {
		// Fix Reolink Doorbell https://github.com/AlexxIT/go2rtc/issues/331
		sender.Handler = pcm.RepackG711(true, sender.Handler)
	}

	sender.HandleRTP(track)

	// keep sender in the backchannel session, so a reconnect of the main session
	// doesn't setup the backchannel media for the second time
	bc.Senders = append(bc.Senders, sender)
	c.backchannel = bc

	// process keepalive and responses from the camera
	go func() {
		_ = bc.Handle()
	}()

	return nil
}
