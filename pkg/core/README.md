## PCM

**RTSP**

- PayloadType=10 - L16/44100/2 - Linear PCM 16-bit big endian
- PayloadType=11 - L16/44100/1 - Linear PCM 16-bit big endian

https://en.wikipedia.org/wiki/RTP_payload_formats

**Apple QuickTime**

- `raw` - 16-bit data is stored in little endian format
- `twos` - 16-bit data is stored in big endian format
- `sowt` - 16-bit data is stored in little endian format
- `in24` - denotes 24-bit, big endian
- `in32` - denotes 32-bit, big endian
- `fl32` - denotes 32-bit floating point PCM
- `fl64` - denotes 64-bit floating point PCM
- `alaw` - denotes A-law logarithmic PCM
- `ulaw` - denotes mu-law logarithmic PCM

https://wiki.multimedia.cx/index.php/PCM

**FFmpeg RTSP**

```
pcm_s16be, 44100 Hz, stereo => 10
pcm_s16be, 48000 Hz, stereo => 96 L16/48000/2
pcm_s16be, 44100 Hz, mono   => 11

pcm_s16le, 48000 Hz, stereo => 96 (b=AS:1536)
pcm_s16le, 44100 Hz, stereo => 96 (b=AS:1411)
pcm_s16le, 16000 Hz, stereo => 96 (b=AS:512)
pcm_s16le, 8000 Hz, stereo  => 96 (b=AS:256)

pcm_s16le, 48000 Hz, mono   => 96 (b=AS:768)
pcm_s16le, 44100 Hz, mono   => 96 (b=AS:705)
pcm_s16le, 16000 Hz, mono   => 96 (b=AS:256)
pcm_s16le, 8000 Hz, mono    => 96 (b=AS:128)
```