# SIP Auto-Answer Consumer

Brings up SIP auto-answer listeners that accept incoming SIP calls and stream audio
from a configured go2rtc stream to the caller. Supports backchannel audio from
the caller back to the camera.

## Configuration

```yaml
sip:
  - port: 5060
    stream: front_door
  - port: 5061
    stream: back_door
```

Each entry in the `sip` array creates an independent SIP listener on its own port.
Calls arriving on that port are answered and connected to the specified stream.

### Port defaults

If `port` is omitted, it defaults to `5060 + index` (5060, 5061, 5062, ...).

### Stream is required

Entries without a `stream` are skipped with a warning.

## Supported codecs

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
3. The RTP endpoint is set up as both a Consumer (camera -> caller)
   and a Producer (caller -> camera backchannel).
4. Transcoding is handled automatically when codecs differ.
5. Stale sessions are reaped after 2 minutes of inactivity.

## Call termination

The consumer handles three ways a call can end:

- **BYE**: The caller sends a BYE request. The consumer responds with 200 OK,
  removes the RTP consumer from the stream, and frees the session.
- **CANCEL**: The caller cancels an in-progress INVITE. Same cleanup as BYE.
- **Timeout**: If no BYE/CANCEL arrives, a background goroutine reaps sessions
  older than 2 minutes, removing the consumer and freeing resources.

## Session cleanup

A background goroutine runs every 30 seconds and removes sessions older than
2 minutes, freeing the associated RTP ports and removing the consumer from
the stream.
