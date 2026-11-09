# RTP Audio Endpoint (no transcoding)

Audio-only RTP endpoint that works as both a **Consumer** (forwards camera audio
to a remote RTP target) and a **Producer** (receives audio from the remote and
feeds it into the stream as backchannel). **No transcoding is performed** — the
stream matcher pairs matching codecs, and RTP packets pass through as-is.

## How it works

### As Consumer (camera → caller)

1. `AddTrack()` is called when the stream has audio to send.
2. A `core.Sender` wraps the camera's RTP packets with new RTP headers and
   sends them directly to the remote address (no transcoding).

### As Producer (caller → camera backchannel)

1. A background `readLoop()` goroutine reads incoming RTP packets.
2. Packets are forwarded as-is to the backchannel receiver.
3. A separate `rtcpReadLoop()` on port+1 handles RTCP for session keepalive.

### Session keepalive

- Dual-socket design: RTP on `port`, RTCP on `port+1`
- Any incoming RTP or RTCP packet fires `OnActivity`
- RTCP Sender Reports (SR) from the caller trigger Receiver Report (RR) responses

## Constructor

```go
func NewRTP(remote string, localPort int) (*RTP, error)
```

- `remote` — target address in `"host:port"` form
- `localPort` — UDP port for RTP (RTCP listens on port+1)

## State isolation

Each `RTP` instance holds its own UDP connections, sequence number / SSRC,
and Sender/Receiver references — safe for multiple independent endpoints.