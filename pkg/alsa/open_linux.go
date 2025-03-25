package alsa

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/AlexxIT/go2rtc/pkg/alsa/device"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func Open(rawURL string) (core.Producer, error) {
	// Example (ffmpeg source compatible):
	// alsa:device?audio=/dev/snd/pcmC0D0p
	// TODO: ?audio=default
	// TODO: ?audio=hw:0,0
	// TODO: &sample_rate=48000&channels=2
	// TODO: &backchannel=1
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	path := u.Query().Get("audio")
	dev, err := device.Open(path)
	if err != nil {
		return nil, err
	}

	if !dev.CheckFormat(device.SNDRV_PCM_FORMAT_S16_LE) {
		_ = dev.Close()
		return nil, errors.New("alsa: format S16LE not supported")
	}

	switch path[len(path)-1] {
	case 'p': // playback
		return newPlayback(dev)
	case 'c': // capture
		return newCapture(dev)
	}

	_ = dev.Close()
	return nil, fmt.Errorf("alsa: unknown path: %s", path)
}
