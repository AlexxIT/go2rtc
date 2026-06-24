# RTP Bidirectional Audio Endpoint

Implements a raw RTP audio endpoint that works as both a **Consumer** (sends
audio from a go2rtc stream to a remote RTP target) and a **Producer** (receives
audio from the remote target and feeds it back into the stream as backchannel).

Used by the SIP auto-answer module (`internal/sip`) and the RTP URL handler
(`internal/streams`).

## How it works

### As Consumer (camera → remote)

1. The remote endpoint registers itself via `AddTrack()`, which is called when
   the stream has audio to send.
2. A `core.Sender` is created with a handler that transcodes the camera's native
   audio codec to the remote codec (PCMA or PCMU) if needed.
3. The transcoded packets are wrapped in RTP headers and sent to the remote
   address via UDP.

### As Producer (remote → camera backchannel)

1. A background `readLoop()` goroutine listens for incoming UDP packets on the
   local port.
2. Incoming RTP packets are unmarshaled and transcoded from the remote codec to
   the camera's expected codec if needed.
3. The transcoded packets are fed into the stream via a `core.Receiver`.

## Codec support

| Remote codec | Camera codec | Transcoding |
|---|---|---|
| PCMA | PCMA | None (passthrough) |
| PCMU | PCMU | None (passthrough) |
| PCMA | PCMU | `pcm.Transcode` |
| PCMU | PCMA | `pcm.Transcode` |
| PCMA/PCMU | PCM/PCML | `pcm.Transcode` |

## Constructor

```go
func NewRTP(remote string, localPort int, remoteCodec *core.Codec) (*RTP, error)
```

- `remote` — target address in `"host:port"` form
- `localPort` — UDP port to listen on for backchannel reception
- `remoteCodec` — the codec used on the remote side (typically PCMA or PCMU)

## State isolation

Each `RTP` instance holds its own:
- UDP connection
- Transcoding function pointers
- Sequence number / SSRC
- Sender and Receiver references

This makes it safe to create multiple independent endpoints, one per SIP call.