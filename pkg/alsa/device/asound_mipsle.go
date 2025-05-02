package device

type unsigned_char = byte
type signed_int = int32
type unsigned_int = uint32
type signed_long = int64
type unsigned_long = uint64
type __u32 = uint32
type void__user = uintptr

const (
	SNDRV_PCM_STREAM_PLAYBACK = 0
	SNDRV_PCM_STREAM_CAPTURE  = 1

	SNDRV_PCM_ACCESS_MMAP_INTERLEAVED    = 0
	SNDRV_PCM_ACCESS_MMAP_NONINTERLEAVED = 1
	SNDRV_PCM_ACCESS_MMAP_COMPLEX        = 2
	SNDRV_PCM_ACCESS_RW_INTERLEAVED      = 3
	SNDRV_PCM_ACCESS_RW_NONINTERLEAVED   = 4

	SNDRV_PCM_FORMAT_S8         = 0
	SNDRV_PCM_FORMAT_U8         = 1
	SNDRV_PCM_FORMAT_S16_LE     = 2
	SNDRV_PCM_FORMAT_S16_BE     = 3
	SNDRV_PCM_FORMAT_U16_LE     = 4
	SNDRV_PCM_FORMAT_U16_BE     = 5
	SNDRV_PCM_FORMAT_S24_LE     = 6
	SNDRV_PCM_FORMAT_S24_BE     = 7
	SNDRV_PCM_FORMAT_U24_LE     = 8
	SNDRV_PCM_FORMAT_U24_BE     = 9
	SNDRV_PCM_FORMAT_S32_LE     = 10
	SNDRV_PCM_FORMAT_S32_BE     = 11
	SNDRV_PCM_FORMAT_U32_LE     = 12
	SNDRV_PCM_FORMAT_U32_BE     = 13
	SNDRV_PCM_FORMAT_FLOAT_LE   = 14
	SNDRV_PCM_FORMAT_FLOAT_BE   = 15
	SNDRV_PCM_FORMAT_FLOAT64_LE = 16
	SNDRV_PCM_FORMAT_FLOAT64_BE = 17
	SNDRV_PCM_FORMAT_MU_LAW     = 20
	SNDRV_PCM_FORMAT_A_LAW      = 21
	SNDRV_PCM_FORMAT_MPEG       = 23

	SNDRV_PCM_IOCTL_PVERSION      = 0x40044100
	SNDRV_PCM_IOCTL_INFO          = 0x41204101
	SNDRV_PCM_IOCTL_HW_REFINE     = 0xc25c4110
	SNDRV_PCM_IOCTL_HW_PARAMS     = 0xc25c4111
	SNDRV_PCM_IOCTL_SW_PARAMS     = 0xc0684113
	SNDRV_PCM_IOCTL_PREPARE       = 0x20004140
	SNDRV_PCM_IOCTL_WRITEI_FRAMES = 0x800c4150
	SNDRV_PCM_IOCTL_READI_FRAMES  = 0x400c4151
)

type snd_pcm_info struct { // size 288
	device           unsigned_int      // offset 0, size 4
	subdevice        unsigned_int      // offset 4, size 4
	stream           signed_int        // offset 8, size 4
	card             signed_int        // offset 12, size 4
	id               [64]unsigned_char // offset 16, size 64
	name             [80]unsigned_char // offset 80, size 80
	subname          [32]unsigned_char // offset 160, size 32
	dev_class        signed_int        // offset 192, size 4
	dev_subclass     signed_int        // offset 196, size 4
	subdevices_count unsigned_int      // offset 200, size 4
	subdevices_avail unsigned_int      // offset 204, size 4
	pad1             [16]unsigned_char
	reserved         [64]unsigned_char // offset 224, size 64
}

type snd_pcm_uframes_t = unsigned_long
type snd_pcm_sframes_t = signed_long

type snd_xferi struct { // size 12
	result snd_pcm_sframes_t // offset 0, size 4
	buf    void__user        // offset 4, size 4
	frames snd_pcm_uframes_t // offset 8, size 4
}

const (
	SNDRV_PCM_HW_PARAM_ACCESS     = 0
	SNDRV_PCM_HW_PARAM_FORMAT     = 1
	SNDRV_PCM_HW_PARAM_SUBFORMAT  = 2
	SNDRV_PCM_HW_PARAM_FIRST_MASK = 0
	SNDRV_PCM_HW_PARAM_LAST_MASK  = 2

	SNDRV_PCM_HW_PARAM_SAMPLE_BITS    = 8
	SNDRV_PCM_HW_PARAM_FRAME_BITS     = 9
	SNDRV_PCM_HW_PARAM_CHANNELS       = 10
	SNDRV_PCM_HW_PARAM_RATE           = 11
	SNDRV_PCM_HW_PARAM_PERIOD_TIME    = 12
	SNDRV_PCM_HW_PARAM_PERIOD_SIZE    = 13
	SNDRV_PCM_HW_PARAM_PERIOD_BYTES   = 14
	SNDRV_PCM_HW_PARAM_PERIODS        = 15
	SNDRV_PCM_HW_PARAM_BUFFER_TIME    = 16
	SNDRV_PCM_HW_PARAM_BUFFER_SIZE    = 17
	SNDRV_PCM_HW_PARAM_BUFFER_BYTES   = 18
	SNDRV_PCM_HW_PARAM_TICK_TIME      = 19
	SNDRV_PCM_HW_PARAM_FIRST_INTERVAL = 8
	SNDRV_PCM_HW_PARAM_LAST_INTERVAL  = 19

	SNDRV_MASK_MAX = 256

	SNDRV_PCM_TSTAMP_NONE   = 0
	SNDRV_PCM_TSTAMP_ENABLE = 1
)

type snd_mask struct { // size 32
	bits [(SNDRV_MASK_MAX + 31) / 32]__u32 // offset 0, size 32
}

type snd_interval struct { // size 12
	min unsigned_int // offset 0, size 4
	max unsigned_int // offset 4, size 4
	bit unsigned_int
}

type snd_pcm_hw_params struct { // size 604
	flags     unsigned_int                                                                           // offset 0, size 4
	masks     [SNDRV_PCM_HW_PARAM_LAST_MASK - SNDRV_PCM_HW_PARAM_FIRST_MASK + 1]snd_mask             // offset 4, size 96
	mres      [5]snd_mask                                                                            // offset 100, size 160
	intervals [SNDRV_PCM_HW_PARAM_LAST_INTERVAL - SNDRV_PCM_HW_PARAM_FIRST_INTERVAL + 1]snd_interval // offset 260, size 144
	ires      [9]snd_interval                                                                        // offset 404, size 108
	rmask     unsigned_int                                                                           // offset 512, size 4
	cmask     unsigned_int                                                                           // offset 516, size 4
	info      unsigned_int                                                                           // offset 520, size 4
	msbits    unsigned_int                                                                           // offset 524, size 4
	rate_num  unsigned_int                                                                           // offset 528, size 4
	rate_den  unsigned_int                                                                           // offset 532, size 4
	fifo_size snd_pcm_uframes_t                                                                      // offset 536, size 4
	reserved  [64]unsigned_char                                                                      // offset 540, size 64
}

type snd_pcm_sw_params struct { // size 104
	tstamp_mode       signed_int        // offset 0, size 4
	period_step       unsigned_int      // offset 4, size 4
	sleep_min         unsigned_int      // offset 8, size 4
	avail_min         snd_pcm_uframes_t // offset 12, size 4
	xfer_align        snd_pcm_uframes_t // offset 16, size 4
	start_threshold   snd_pcm_uframes_t // offset 20, size 4
	stop_threshold    snd_pcm_uframes_t // offset 24, size 4
	silence_threshold snd_pcm_uframes_t // offset 28, size 4
	silence_size      snd_pcm_uframes_t // offset 32, size 4
	boundary          snd_pcm_uframes_t // offset 36, size 4
	proto             unsigned_int      // offset 40, size 4
	tstamp_type       unsigned_int      // offset 44, size 4
	reserved          [56]unsigned_char // offset 48, size 56
}
