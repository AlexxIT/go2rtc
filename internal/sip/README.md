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

## How it works

1. Each consumer starts a UDP listener on its configured port.
2. When an `INVITE` arrives, the consumer:
   - Looks up the configured stream
   - Parses the SDP offer to find the caller's remote RTP address
   - Negotiates PCMA or PCMU codec
   - Creates an `rtp.RTP` endpoint on a dynamically allocated local UDP port
   - Responds with `200 OK` containing an SDP answer
   - Adds the RTP endpoint as a consumer to the stream (camera → caller)
3. The RTP endpoint is set up as both a Consumer (audio from camera → caller)
   and a Producer (audio from caller → camera backchannel).
4. Transcoding (PCMA ↔ PCMU, PCM, PCML) is handled automatically.
5. Stale sessions are reaped after 2 minutes of inactivity.

## Session cleanup

A background goroutine runs every 30 seconds and removes sessions older than
2 minutes, freeing the associated RTP ports and removing the consumer from
the stream.