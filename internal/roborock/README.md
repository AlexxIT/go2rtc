# Roborock

[`new in v1.3.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)

This source type supports Roborock vacuums with cameras. Known working models:

- **Roborock S6 MaxV** - only video (the vacuum has no microphone)
- **Roborock S7 MaxV** - video and two-way audio
- **Roborock Qrevo MaxV** - video and two-way audio
- **Roborock Qrevo Edge 2 (QX Revo Ultra 2)** - video and two-way audio (verified on firmware 02.15.44)

## Configuration

This source supports loading Roborock credentials from the Home Assistant [custom integration](https://github.com/humbertogontijo/homeassistant-roborock) or the [core integration](https://www.home-assistant.io/integrations/roborock). Otherwise, you need to log in to your Roborock account (MiHome account is not supported). Go to go2rtc WebUI > Add webpage. Copy the `roborock://...` source for your vacuum and paste it into your `go2rtc.yaml` config.

If you have a pattern PIN for your vacuum, add it as a numeric PIN (lines: 123, 456, 789) to the end of the `roborock` link.

## Notes on two-way audio (Qrevo Edge 2 / QX Revo Ultra 2)

Verified live on firmware 02.15.44:

- Audio is gated by the device RPC `start_voice_chat`: the microphone feed runs with `record: true`, and audio to the speaker additionally requires `play: true`. `set_voice_chat_volume` is accepted during the session on this model.
- The intercom is half-duplex: while the speaker is playing, the microphone feed is muted (digital silence in stretches shaped by the speech), so a loopback recording made during playback contains the room sound before and after the playback, but not the played audio itself.
- Disconnecting the client does not end the device-side voice session: the device keeps reporting a connected preview client until the session is closed with `STOP_CAMERA_PREVIEW`. End sessions with the stop command and verify the connection status shows no connected client before starting another one.
