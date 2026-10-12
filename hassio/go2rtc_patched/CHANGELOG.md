# Changelog

## 1.9.14-tapo17

- `backchannel_wake` also waits if the camera was woken up by something else

## 1.9.14-tapo16

- `backchannel_wake`: later talk session for a battery camera that was just woken up, a D235 freezes otherwise

## 1.9.14-tapo15

- The audio codec of the camera (Opus or PCMA) is detected, mains powered cameras keep PCMA
- Based on the current go2rtc master

## 1.9.14-tapo14

- Mixing only for connections that support it, other cameras replace the running playback as before
- Stop of the playback drops the queued audio at once
- Limiter for the mixed audio
- Unused camera connections are closed also while a playback runs
- Diagnostics use the log level of the module `tapo`
- aarch64 build (not tested)

## 1.9.14-tapo13

- Several playbacks at the same time are mixed into one talk session

## 1.9.14-tapo10

- A leaving consumer (ex. a snapshot) doesn't stop a running playback

## 1.9.14-tapo7

- No crash on a broken encrypted packet

## 1.9.14-tapo5

- H265 video over the Tapo protocol

## 1.9.14-tapo3

- Paced backchannel for playback with FFmpeg, pre-fill and lead-in
