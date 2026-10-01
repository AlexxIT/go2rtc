# AV1

AV1 carries its codec parameters in a sequence header OBU inside the bitstream, not in the SDP. Encoders repeat that OBU in front of every keyframe, so consumers that need an `av1C` record (MP4, MSE, HLS) have to wait for one before they can build an init segment.

Depayloading uses pion's `AV1Depacketizer`. It rewrites `obu_size` while reassembling, so a temporal unit spliced together across a packet loss still parses as a valid keyframe. `RTPDepay` therefore drops such a unit and resumes at the next marker instead of mid unit.

## Useful Links

- [AV1 Bitstream & Decoding Process Specification](https://aomediacodec.github.io/av1-spec/)
- [RTP Payload Format For AV1 (RFC 9583)](https://www.rfc-editor.org/rfc/rfc9583)
- [AV1 Codec ISO Media File Format Binding](https://aomediacodec.github.io/av1-isobmff/)
- [AV1 levels](https://aomediacodec.github.io/av1-spec/#levels)
- [Codecs parameter for AV1 (MDN)](https://developer.mozilla.org/en-US/docs/Web/Media/Guides/Formats/codecs_parameter#av1)
- [Enhanced RTMP v2](https://github.com/veovera/enhanced-rtmp/blob/main/docs/enhanced/enhanced-rtmp-v2.md)
