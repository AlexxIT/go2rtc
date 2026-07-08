# UniFi Protect

UniFi Protect talkback is supported as a native go2rtc audio backchannel.

## Configuration

```yaml
streams:
  gate:
    - rtspx://192.168.1.1:7441/<alias>#backchannel=0
    - ffmpeg:gate#audio=opus
    - unifi-talkback:https://192.168.1.1?camera_id=<id>&api_key=<secret>
```

The `unifi-talkback:` source uses the UniFi Protect public API:

- `GET /proxy/protect/integration/v1/cameras/{camera_id}` checks whether the camera has a speaker
- `POST /proxy/protect/integration/v1/cameras/{camera_id}/talkback-session` starts a talkback session

Normal `video+audio` viewing does not activate talkback. go2rtc opens the UniFi talkback session only after a client sends microphone RTP into the audio backchannel.

Push-to-talk clients should start sending microphone audio when talk begins and stop or disconnect that microphone send session when talk ends. go2rtc closes the FFmpeg RTP output when the backchannel producer stops.

Only UniFi talkback sessions that report `codec: "opus"` are supported.
