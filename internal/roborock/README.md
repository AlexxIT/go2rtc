# Roborock

[`new in v1.3.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.3.0)

This source type supports Roborock vacuums with cameras. Known working models:

- **Roborock S6 MaxV** - only video (the vacuum has no microphone)
- **Roborock S7 MaxV** - video and two-way audio
- **Roborock Qrevo MaxV** - video and two-way audio

## Configuration

This source supports loading Roborock credentials from the Home Assistant [custom integration](https://github.com/humbertogontijo/homeassistant-roborock) or the [core integration](https://www.home-assistant.io/integrations/roborock). Otherwise, you need to log in to your Roborock account (MiHome account is not supported). Go to go2rtc WebUI > Add webpage. Copy the `roborock://...` source for your vacuum and paste it into your `go2rtc.yaml` config.

If you have a pattern PIN for your vacuum, add it as a numeric PIN (lines: 123, 456, 789) to the end of the `roborock` link.
