# Hardware

You **DON'T** need hardware acceleration if:

- you're not using the [FFmpeg source](../README.md)
- you're using only `#video=copy` for the FFmpeg source
- you're using only `#audio=...` (any audio) transcoding for the FFmpeg source

You **NEED** hardware acceleration if you're using `#video=h264`, `#video=h265`, `#video=mjpeg` (video) transcoding.

## Important

- Acceleration is disabled by default because it can be unstable (this may change in the future)
- go2rtc can automatically detect supported hardware acceleration if enabled
- go2rtc will enable hardware decoding only if hardware encoding is supported
- go2rtc will use the same GPU for decoder and encoder
- Intel and AMD will switch to a software decoder if the input codec isn't supported by the hardware decoder
- NVIDIA will fail if the input codec isn't supported by the hardware decoder
- Raspberry Pi always uses a software decoder

```yaml
streams:
  # auto select hardware encoder
  camera1_hw: ffmpeg:rtsp://rtsp:12345678@192.168.1.123/av_stream/ch0#video=h264#hardware

  # manual select hardware encoder (vaapi, cuda, v4l2m2m, dxva2, videotoolbox)
  camera1_vaapi: ffmpeg:rtsp://rtsp:12345678@192.168.1.123/av_stream/ch0#video=h264#hardware=vaapi
```

## Docker and Hass Addon

There are two versions of the Docker container and Hass Add-on:

- Latest (Alpine) supports hardware acceleration for Intel iGPU (CPU with graphics) and Raspberry Pi.
- Hardware (Debian 12) supports Intel iGPU, AMD GPU, NVIDIA GPU.

## Intel iGPU

**Supported on:** Windows binary, Linux binary, Docker, Hass Addon.

If you have an Intel Sandy Bridge (2011) CPU with graphics, you already have hardware decoding/encoding support for `AVC/H.264`.

If you have an Intel Skylake (2015) CPU with graphics, you already have hardware decoding/encoding support for `AVC/H.264`, `HEVC/H.265` and `MJPEG`.

Read more [here](https://en.wikipedia.org/wiki/Intel_Quick_Sync_Video#Hardware_decoding_and_encoding) and [here](https://en.wikipedia.org/wiki/Intel_Graphics_Technology#Capabilities_(GPU_video_acceleration)).

Linux and Docker:

- It may be important to have a recent OS and Linux kernel. For example, on my **Debian 10 (kernel 4.19)** it did not work, but after updating to **Debian 11 (kernel 5.10)** everything was fine.
- If you run into trouble, check that you have the `/dev/dri/` folder on your host.

Docker users should add the `--privileged` option to the container for access to the hardware.

**PS.** Supported via [VAAPI](https://trac.ffmpeg.org/wiki/Hardware/VAAPI) engine on Linux and [DXVA2+QSV](https://trac.ffmpeg.org/wiki/Hardware/QuickSync) engine on Windows.

## AMD GPU

*I don't have the hardware to test this!!!*

**Supported on:** Linux binary, Docker, Hass Addon.

Docker users should install: `alexxit/go2rtc:master-hardware`. Docker users should add the `--privileged` option to the container for access to the hardware.

Hass Addon users should install **go2rtc master hardware** version.

**PS.** Supported via [VAAPI](https://trac.ffmpeg.org/wiki/Hardware/VAAPI) engine.

## NVIDIA GPU

**Supported on:** Windows binary, Linux binary, Docker.

Docker users should install: `alexxit/go2rtc:master-hardware`.

Read more [here](https://docs.frigate.video/configuration/hardware_acceleration) and [here](https://jellyfin.org/docs/general/administration/hardware-acceleration/#nvidia-hardware-acceleration-on-docker-linux).

**PS.** Supported via [CUDA](https://trac.ffmpeg.org/wiki/HWAccelIntro#CUDANVENCNVDEC) engine.

## Raspberry Pi 3

**Supported on:** Linux binary, Docker, Hass Addon.

I don't recommend using transcoding on the Raspberry Pi 3. It's extremely slow, even with hardware acceleration. Also, it may fail when transcoding a 2K+ stream.

## Raspberry Pi 4

*I don't have the hardware to test this!!!*

**Supported on:** Linux binary, Docker, Hass Addon.

**PS.** Supported via [v4l2m2m](https://lalitm.com/hw-encoding-raspi/) engine.

## macOS

In my tests, transcoding is faster on the M1 CPU than on the M1 GPU. Transcoding time on the M1 CPU is better than any Intel iGPU and comparable to an NVIDIA RTX 2070.

**PS.** Supported via [videotoolbox](https://trac.ffmpeg.org/wiki/HWAccelIntro#VideoToolbox) engine.

## Rockchip

- It's important to use a custom FFmpeg build with Rockchip support from [@nyanmisaka](https://github.com/nyanmisaka/ffmpeg-rockchip)
  - Static binaries from [@MarcA711](https://github.com/MarcA711/Rockchip-FFmpeg-Builds/releases/)
- It's important to have Linux kernel 5.10 or 6.1

**Tested**

- [Orange Pi 3B](https://www.armbian.com/orangepi3b/) with Armbian 6.1, supports transcoding H.264, H.265, MJPEG
