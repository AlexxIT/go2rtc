# go2rtc (Tapo patch)

## Installation

Add the repository `https://git.btaubmann.de/tux/go2rtc` to the add-on store of Home Assistant.

1. Stop the official go2rtc add-on, if it is installed. Both use the ports 1984, 8554 and 8555.
2. Install and start this add-on. Home Assistant builds it from the source, this takes a few minutes and needs
   about 3 GB of free disk space.
3. The config is the same file as before: `/config/go2rtc.yaml`.

Only one go2rtc add-on can run at the same time.

## Tapo camera with speaker

```yaml
streams:
  # video and two-way audio of the camera cards
  doorbell:
    - tapo://cloud-password@192.168.1.123
  # only for the media player
  doorbell_playback:
    - tapo://cloud-password@192.168.1.123?backchannel_pace=120&backchannel_prefill=400&backchannel_leadin=1500

ffmpeg:
  # no spaces are allowed in the source of the API, so use a preset
  doorbell_tts: "-c:a pcm_alaw -ar:a 8000 -ac:a 1"

log:
  tapo: debug  # optional, statistics of every playback
```

| Param                 | Range   | Default | Description                                                                         |
|-----------------------|---------|---------|-------------------------------------------------------------------------------------|
| `backchannel_pace`    | 0..2000 | off     | Paced mode for playback. Value: ms of audio collected before the start.             |
| `backchannel_prefill` | 0..2000 | 80      | Silence in ms sent at once before the audio, the cushion for the network jitter.    |
| `backchannel_leadin`  | 0..5000 | 0       | Silence in ms sent in real time before the audio, until the speaker is switched on. |

The paced mode and the lead-in delay everything that is sent to the camera. Use a separate stream for the media
player, so two-way audio of a camera card is not delayed. The camera supports only one talk session: two-way audio
and a playback at the same time don't work with separate streams.

## Media player in Home Assistant

With the [WebRTC Camera](https://github.com/AlexxIT/WebRTC) integration:

```yaml
media_player:
  - platform: webrtc
    name: doorbell
    stream: doorbell_playback
    audio: doorbell_tts
```

`media_player.play_media` and `tts.speak` play on the speaker of the camera. A second playback is mixed with the
running one. `media_player.media_stop` stops everything.

## Tested with

- Tapo D235 (doorbell) on battery and on power, amd64

The aarch64 build is not tested.
