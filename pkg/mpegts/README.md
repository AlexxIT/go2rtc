## PTS/DTS/CTS

```
if DTS == 0 {
    // for I and P frames
	packet.Timestamp = PTS (presentation time)
} else {
    // for B frames
    packet.Timestamp = DTS (decode time)
    CTS = PTS-DTS (composition time)
}
```

- MPEG-TS container uses PTS and optional DTS.
- MP4 container uses DTS and CTS
- RTP container uses PTS

## MPEG-TS

FFmpeg:
- PMTID=4096
- H264: PESID=256, StreamType=27, StreamID=224
- H265: PESID=256, StreamType=36, StreamID=224
- AAC: PESID=257, StreamType=15, StreamID=192

Tapo:
- PMTID=18
- H264: PESID=68, StreamType=27, StreamID=224
- AAC: PESID=69, StreamType=144, StreamID=192

## Useful links

- https://github.com/theREDspace/video-onboarding/blob/main/MPEGTS%20Knowledge.md
- https://en.wikipedia.org/wiki/MPEG_transport_stream
- https://en.wikipedia.org/wiki/Program-specific_information
