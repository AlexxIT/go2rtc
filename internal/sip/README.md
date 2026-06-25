# SIP Auto-Answer Consumer

Brings up SIP auto-answer listeners that accept incoming SIP calls and stream audio
(and optionally video) from a configured go2rtc stream to the caller. Supports
backchannel audio from the caller back to the camera.

## Configuration

```yaml
sip:
  - port: 5060
    stream: front_door
    video: yes
  - port: 5061
    stream: back_door
```

Each entry in the `sip` array creates an independent SIP listener on its own port.
Calls arriving on that port are answered and connected to the specified stream.

### Port is required

Every SIP entry must have a `port`. Entries without a port are skipped with a warning.

### Stream is required

Entries without a `stream` are skipped with a warning.

### Video (optional)

Set `video: yes` to include the stream's video in the SDP answer. The camera's
native video codec is offered as-is — no transcoding is performed. If the camera
has no video output, the video port is simply not added to the SDP answer and
audio continues to work.

Default: `video: no` (audio only).

## Supported codecs

### Audio

The SIP consumer answers with the camera's native audio codec so the SIP link
uses the same format in both directions (send and receive). This avoids
transcoding when possible.

| Camera codec | Answer codec | Direction |
|-------------|-------------|-----------|
| Opus        | Opus        | Pass-through both ways |
| G722        | G722        | Pass-through both ways |
| PCMA        | PCMA        | Pass-through both ways |
| PCMU        | PCMU        | Pass-through both ways |
| PCM/PCML    | PCMA        | Transcoded to PCMA |
| AAC         | PCMA/PCMU   | Rejected (488) |

If the camera codec is unknown or unsupported, the consumer falls back to
PCMA or PCMU (whichever the caller supports). If the caller supports neither,
the INVITE is rejected with 488 Codec Mismatch.

### Video

Video is passed through using the camera's native codec with no transcoding.
Video direction is camera → caller only (sendonly).

| Camera codec | Answer codec | Direction |
|-------------|-------------|-----------|
| H264        | H264        | Camera → caller only |
| H265        | H265        | Camera → caller only |

## How it works

1. Each consumer starts a UDP listener on its configured port.
2. When an `INVITE` arrives, the consumer:
   - Looks up the configured stream
   - Parses the SDP offer to find the caller's remote RTP address
   - Detects the best SIP-compatible audio codec from the stream (via BestSIPCodec)
   - Answers with the camera's codec (or PCMA/PCMU fallback)
   - Creates an `rtp.RTP` endpoint on a dynamically allocated local UDP port
   - Responds with `200 OK` containing an SDP answer
   - Adds the RTP endpoint as a consumer to the stream (camera -> caller)
3. If `video: yes`, the consumer also:
   - Discovers the stream's video codec (via BestVideoCodec)
   - Allocates a second UDP port for video RTP
   - Includes an `m=video` line in the SDP answer
   - Creates a second `rtp.RTP` endpoint for video (camera -> caller only)
4. The audio RTP endpoint is set up as both a Consumer (camera -> caller)
   and a Producer (caller -> camera backchannel).
5. The video RTP endpoint is set up as a Consumer only (camera -> caller).
6. Transcoding is handled automatically when audio codecs differ. Video is never
   transcoded — the stream must have a matching video codec.
7. For video calls, RTCP Receiver Reports are sent to keep the remote happy.
8. If no RTP or RTCP packets arrive from the remote for 1 minute (network drop,
   device crash without BYE), the session is reaped and consumers are removed.

## Call termination

A background goroutine checks every 30 seconds. A session ends when:

- **BYE**: The caller sends a BYE request. The consumer responds with 200 OK,
  removes the audio and video RTP consumers from the stream, and frees the session.
- **CANCEL**: The caller cancels an in-progress INVITE. Same cleanup as BYE.
- **Silent timeout**: No RTP or RTCP from the remote for 1 minute. The session
  is reaped and both audio and video consumers are removed from the stream.
  Active calls are unaffected — any incoming packet resets the timer.
