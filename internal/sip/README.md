# SIP Auto-Answer Consumer (audio-only, no transcoding)

Brings up SIP auto-answer listeners that accept incoming SIP calls and stream
audio from a configured go2rtc stream to the caller. Supports backchannel audio
from the caller back to the camera. **No transcoding is performed** — the user
is responsible for ensuring matching codecs in both directions.

## Configuration

```yaml
sip:
  rtp_port_range: "31000-31100"   # optional, default 31000-31100 (50 calls)
  host_ip: "203.0.113.5"          # optional, overrides SDP c= line (Docker/NAT)
  consumers:
    - port: 5060
      stream: front_door
    - port: 5061
      stream: back_door
```

Each entry in the `consumers` array creates an independent SIP listener on its own port.

### Port is required

Every SIP entry must have a `port`. Entries without a port are skipped with a warning.

### Stream is required

Entries without a `stream` are skipped with a warning.

### RTP port range (optional)

`rtp_port_range` defines the global pool of RTP ports shared across all SIP
consumers. Format: `"min-max"` (e.g., `"31000-31100"`). Must span at least 4
ports. Default: `31000-31100` (100 ports = 50 simultaneous calls).

This is critical for Docker bridge mode — only this range needs to be forwarded:
```bash
docker run -p 5060-5061:5060-5061/udp -p 31000-31100:31000-31100/udp ...
```

### Host IP override (optional)

`host_ip` overrides the IP address advertised in the SDP `c=` line and Contact
header. Use this when running behind Docker NAT or any NAT where the container's
internal IP is not reachable by the caller.

When unset, the IP is auto-detected by dialing the remote address (works for
host network mode and LAN scenarios).

Set `host_ip` to your Docker host's public IP (or any IP the caller can reach)
when using bridge mode.

## Codec negotiation

go2rtc negotiates the best common audio codec between the camera and the caller.
No transcoding is performed — the user handles that via `exec`/`ffmpeg` on the
stream if needed.

| Main audio exists? | Backchannel exists? | Answer direction | Notes |
|---|---|---|---|
| Yes (common codec found) | Yes/No | `sendrecv` | Main codec + any backchannel codecs advertised |
| No (no common codec) | — | `recvonly` | All codecs advertised; keepalive via RTCP |

- **sendrecv**: Both directions use the same m=audio line. The caller receives
  the camera's audio and can send backchannel audio. RTP/RTCP from the caller
  keeps the session alive.
- **recvonly**: Only the caller can send audio (keepalive path). No audio flows
  from the camera if no common codec is found.

## Audio-only

This is an audio-only implementation. Video is not supported. Any video streams
from the camera are ignored.

## Session management

- Activity timer starts when the session is created.
- **All RTP packets** from the caller reset the timer.
- **All RTCP packets** (SR/RR/compound) on RTP or RTCP port reset the timer.
- RTCP Receiver Reports are sent in response to incoming Sender Reports.
- 1-minute silent timeout reaps dead sessions regardless of direction.

### Session teardown

- **BYE**: Cleanup and respond 200 OK.
- **CANCEL**: Cleanup and respond.
- **Timeout**: 30-second check loop reaps sessions silent for >1 minute.

## Limitations

- **No transcoding**: The same codec is used for both directions. Use
  `exec`/`ffmpeg` on the stream if format conversion is needed.
- **No video**: This is audio-only.
- **No AAC**: AAC is not supported. The user must not configure AAC streams with
  this module.
- **User responsibility**: It is up to the user to provide matching in and out
  codecs, and to not provide AAC.
- **LAN scope**: No authentication is included. Use within a trusted network or
  behind a PBX. Docker bridge mode with port forwarding is supported.
