package device

import (
	"fmt"
	"syscall"
	"unsafe"
)

type Device struct {
	fd   uintptr
	path string

	hwparams   snd_pcm_hw_params
	frameBytes int // sample size * channels
}

func Open(path string) (*Device, error) {
	// important to use nonblock because can get lock
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}

	// important to remove nonblock because better to handle reads and writes
	if err = syscall.SetNonblock(fd, false); err != nil {
		return nil, err
	}

	d := &Device{fd: uintptr(fd), path: path}
	d.init()

	// load all supported formats, channels, rates, etc.
	if err = ioctl(d.fd, SNDRV_PCM_IOCTL_HW_REFINE, &d.hwparams); err != nil {
		_ = d.Close()
		return nil, err
	}

	d.setMask(SNDRV_PCM_HW_PARAM_ACCESS, SNDRV_PCM_ACCESS_RW_INTERLEAVED)

	return d, nil
}

func (d *Device) Close() error {
	return syscall.Close(int(d.fd))
}

func (d *Device) IsCapture() bool {
	// path: /dev/snd/pcmC0D0c, where p - playback, c - capture
	return d.path[len(d.path)-1] == 'c'
}

type Info struct {
	Card      int
	Device    int
	SubDevice int
	Stream    int
	ID        string
	Name      string
	SubName   string
}

func (d *Device) Info() (*Info, error) {
	var info snd_pcm_info
	if err := ioctl(d.fd, SNDRV_PCM_IOCTL_INFO, &info); err != nil {
		return nil, err
	}
	return &Info{
		Card:      int(info.card),
		Device:    int(info.device),
		SubDevice: int(info.subdevice),
		Stream:    int(info.stream),
		ID:        str(info.id[:]),
		Name:      str(info.name[:]),
		SubName:   str(info.subname[:]),
	}, nil
}

func (d *Device) CheckFormat(format byte) bool {
	return d.checkMask(SNDRV_PCM_HW_PARAM_FORMAT, uint32(format))
}

func (d *Device) ListFormats() (formats []byte) {
	for i := byte(0); i <= 28; i++ {
		if d.CheckFormat(i) {
			formats = append(formats, i)
		}
	}
	return
}

func (d *Device) RangeRates() (uint32, uint32) {
	return d.getInterval(SNDRV_PCM_HW_PARAM_RATE)
}

func (d *Device) RangeChannels() (byte, byte) {
	minCh, maxCh := d.getInterval(SNDRV_PCM_HW_PARAM_CHANNELS)
	return byte(minCh), byte(maxCh)
}

func (d *Device) GetRateNear(rate uint32) uint32 {
	r1, r2 := d.RangeRates()
	if rate < r1 {
		return r1
	}
	if rate > r2 {
		return r2
	}
	return rate
}

func (d *Device) GetChannelsNear(channels byte) byte {
	c1, c2 := d.RangeChannels()
	if channels < c1 {
		return c1
	}
	if channels > c2 {
		return c2
	}
	return channels
}

const bufferSize = 4096

func (d *Device) SetHWParams(format byte, rate uint32, channels byte) error {
	d.setInterval(SNDRV_PCM_HW_PARAM_CHANNELS, uint32(channels))
	d.setInterval(SNDRV_PCM_HW_PARAM_RATE, rate)
	d.setMask(SNDRV_PCM_HW_PARAM_FORMAT, uint32(format))
	//d.setMask(SNDRV_PCM_HW_PARAM_SUBFORMAT, 0)

	// important for smooth playback
	d.setInterval(SNDRV_PCM_HW_PARAM_BUFFER_SIZE, bufferSize)
	//d.setInterval(SNDRV_PCM_HW_PARAM_PERIOD_SIZE, 2000)

	if err := ioctl(d.fd, SNDRV_PCM_IOCTL_HW_PARAMS, &d.hwparams); err != nil {
		return fmt.Errorf("[alsa] set hw_params: %w", err)
	}

	_, i := d.getInterval(SNDRV_PCM_HW_PARAM_FRAME_BITS)
	d.frameBytes = int(i / 8)

	_, periods := d.getInterval(SNDRV_PCM_HW_PARAM_PERIODS)
	_, periodSize := d.getInterval(SNDRV_PCM_HW_PARAM_PERIOD_SIZE)
	threshold := snd_pcm_uframes_t(periods * periodSize) // same as bufferSize

	swparams := snd_pcm_sw_params{
		//tstamp_mode: SNDRV_PCM_TSTAMP_ENABLE,
		period_step:    1,
		avail_min:      1, // start as soon as possible
		stop_threshold: threshold,
	}

	if d.IsCapture() {
		swparams.start_threshold = 1
	} else {
		swparams.start_threshold = threshold
	}

	if err := ioctl(d.fd, SNDRV_PCM_IOCTL_SW_PARAMS, &swparams); err != nil {
		return fmt.Errorf("[alsa] set sw_params: %w", err)
	}

	if err := ioctl(d.fd, SNDRV_PCM_IOCTL_PREPARE, nil); err != nil {
		return fmt.Errorf("[alsa] prepare: %w", err)
	}

	return nil
}

func (d *Device) Write(b []byte) (n int, err error) {
	xfer := &snd_xferi{
		buf:    uintptr(unsafe.Pointer(&b[0])),
		frames: snd_pcm_uframes_t(len(b) / d.frameBytes),
	}
	err = ioctl(d.fd, SNDRV_PCM_IOCTL_WRITEI_FRAMES, xfer)
	if err == syscall.EPIPE {
		// auto handle underrun state
		// https://stackoverflow.com/questions/59396728/how-to-properly-handle-xrun-in-alsa-programming-when-playing-audio-with-snd-pcm
		err = ioctl(d.fd, SNDRV_PCM_IOCTL_PREPARE, nil)
	}
	n = int(xfer.result) * d.frameBytes
	return
}

func (d *Device) Read(b []byte) (n int, err error) {
	xfer := &snd_xferi{
		buf:    uintptr(unsafe.Pointer(&b[0])),
		frames: snd_pcm_uframes_t(len(b) / d.frameBytes),
	}
	err = ioctl(d.fd, SNDRV_PCM_IOCTL_READI_FRAMES, xfer)
	n = int(xfer.result) * d.frameBytes
	return
}

func (d *Device) init() {
	for i := range d.hwparams.masks {
		d.hwparams.masks[i].bits[0] = 0xFFFFFFFF
		d.hwparams.masks[i].bits[1] = 0xFFFFFFFF
	}
	for i := range d.hwparams.intervals {
		d.hwparams.intervals[i].max = 0xFFFFFFFF
	}

	d.hwparams.rmask = 0xFFFFFFFF
	d.hwparams.cmask = 0
	d.hwparams.info = 0xFFFFFFFF
}

func (d *Device) setInterval(param, val uint32) {
	d.hwparams.intervals[param-SNDRV_PCM_HW_PARAM_FIRST_INTERVAL].min = val
	d.hwparams.intervals[param-SNDRV_PCM_HW_PARAM_FIRST_INTERVAL].max = val
	d.hwparams.intervals[param-SNDRV_PCM_HW_PARAM_FIRST_INTERVAL].bit = 0b0100 // integer
}

func (d *Device) setIntervalMin(param, val uint32) {
	d.hwparams.intervals[param-SNDRV_PCM_HW_PARAM_FIRST_INTERVAL].min = val
}

func (d *Device) getInterval(param uint32) (uint32, uint32) {
	return d.hwparams.intervals[param-SNDRV_PCM_HW_PARAM_FIRST_INTERVAL].min,
		d.hwparams.intervals[param-SNDRV_PCM_HW_PARAM_FIRST_INTERVAL].max
}

func (d *Device) setMask(mask, val uint32) {
	d.hwparams.masks[mask].bits[0] = 0
	d.hwparams.masks[mask].bits[1] = 0
	d.hwparams.masks[mask].bits[val>>5] = 1 << (val & 0x1F)
}

func (d *Device) checkMask(mask, val uint32) bool {
	return d.hwparams.masks[mask].bits[val>>5]&(1<<(val&0x1F)) > 0
}
