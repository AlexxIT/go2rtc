# WebSocket

Endpoint: `/api/ws`

Query parameters:

- `src` (required) - Stream name

### WebRTC

By default, closing the signalling WebSocket does not close an established
WebRTC peer. Clients normally close it with `RTCPeerConnection.close()`.

Gateways that need to revoke a media session by closing its signalling
connection can opt in before sending an offer:

```json
{"type":"webrtc/session","value":"close-on-disconnect-v1"}
```

Wait for the identical acknowledgement before sending `webrtc/offer` or
`webrtc`. If it is not acknowledged, do not assume this lifetime is supported.
Other values return an error without enabling the option. The option applies
to subsequent offers on this WebSocket, including `dst` producers and audio
backchannels. Once enabled, it cannot be disabled on that connection.

When go2rtc detects that this WebSocket has closed, it closes the associated
peers and removes their consumers/producers from the stream. Peers on other
WebSockets and clients that do not opt in keep their existing behaviour.
A gateway should dedicate one WebSocket to each independently revocable peer,
keep it open for the lifetime of that peer, and control access to go2rtc itself;
this option does not authenticate clients. A network failure takes effect when
the WebSocket disconnection is detected, rather than imposing a media timeout.

Request SDP:

```json
{"type":"webrtc/offer","value":"v=0\r\n..."}
```

Response SDP:

```json
{"type":"webrtc/answer","value":"v=0\r\n..."}
```

Request/response candidate:

- empty value also allowed and optional

```json
{"type":"webrtc/candidate","value":"candidate:3277516026 1 udp 2130706431 192.168.1.123 54321 typ host"}
```

### MSE

Request:

- codecs list optional

```json
{"type":"mse","value":"avc1.640029,avc1.64002A,avc1.640033,hvc1.1.6.L153.B0,mp4a.40.2,mp4a.40.5,flac,opus"}
```

Response:

```json
{"type":"mse","value":"video/mp4; codecs=\"avc1.64001F,mp4a.40.2\""}
```

### HLS

Request:

```json
{"type":"hls","value":"avc1.640029,avc1.64002A,avc1.640033,hvc1.1.6.L153.B0,mp4a.40.2,mp4a.40.5,flac"}
```

Response:

- you MUST rewrite full HTTP path to `http://192.168.1.123:1984/api/hls/playlist.m3u8`

```json
{"type":"hls","value":"#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000000,CODECS=\"avc1.64001F,mp4a.40.2\"\nhls/playlist.m3u8?id=DvmHdd9w"}
```

### MJPEG

Request/response:

```json
{"type":"mjpeg"}
```
