# Wyoming

This module provide [Wyoming Protocol](https://www.home-assistant.io/integrations/wyoming/) support to create local voice assistants using [Home Assistant](https://www.home-assistant.io/).

- go2rtc can act as [Wyoming Satellite](https://github.com/rhasspy/wyoming-satellite)
- go2rtc can act as [Wyoming External Microphone](https://github.com/rhasspy/wyoming-mic-external)
- go2rtc can act as [Wyoming External Sound](https://github.com/rhasspy/wyoming-snd-external)
- any supported audio source with PCM codec can be used as audio input
- any supported two-way audio source with PCM codec can be used as audio output
- any desktop/server microphone/speaker can be used as two-way audio source
  - supported any OS via FFmpeg or any similar software
  - supported Linux via alsa source
- you can change the behavior using the built-in scripting engine

## Typical Voice Pipeline

1. Audio stream (MIC)
   - any audio source with PCM codec support (include PCMA/PCMU)
2. Voice Activity Detector (VAD)
3. Wake Word (WAKE)
   - [OpenWakeWord](https://www.home-assistant.io/voice_control/create_wake_word/)
4. Speech-to-Text (STT)
   - [Whisper](https://github.com/home-assistant/addons/blob/master/whisper/README.md) 
   - [Vosk](https://github.com/rhasspy/hassio-addons/blob/master/vosk/README.md)
5. Conversation agent (INTENT)
   - [Home Assistant](https://www.home-assistant.io/integrations/conversation/)
6. Text-to-speech (TTS)
   - [Google Translate](https://www.home-assistant.io/integrations/google_translate/)
   - [Piper](https://github.com/home-assistant/addons/blob/master/piper/README.md)
7. Audio stream (SND)
   - any source with two-way audio (backchannel) and PCM codec support (include PCMA/PCMU)

You can use a large number of different projects for WAKE, STT, INTENT and TTS thanks to the Home Assistant.

And you can use a large number of different technologies for MIC and SND thanks to Go2rtc.

## Configuration

You can optionally specify WAKE service. So go2rtc will start transmitting audio to Home Assistant only after WAKE word. If the WAKE service cannot be connected to or not specified - go2rtc will pass all audio to Home Assistant. In this case WAKE service must be configured in your Voice Assistant pipeline.

You can optionally specify VAD threshold. So go2rtc will start transmitting audio to WAKE service only after some audio noise.

Your stream must support audio transmission in PCM codec (include PCMA/PCMU).

```yaml
wyoming:
  stream_name_from_streams_section:
    listen: :10700 
    name: "My Satellite"                # optional name
    wake_uri: tcp://192.168.1.23:10400  # optional WAKE service
    vad_threshold: 1                    # optional VAD threshold (from 0.1 to 3.5)
```

Home Assistant -> Settings -> Integrations -> Add -> Wyoming Protocol -> Host + Port from `go2rtc.yaml`

Select one or multiple wake words:
```yaml
wake_uri: tcp://192.168.1.23:10400?name=alexa_v0.1&name=hey_jarvis_v0.1&name=hey_mycroft_v0.1&name=hey_rhasspy_v0.1&name=ok_nabu_v0.1
```

## Events

You can add wyoming event handling using the [expr](https://github.com/AlexxIT/go2rtc/blob/master/internal/expr/README.md) language. For example, to pronounce TTS on some media player from HA.

Turn on the logs to see what kind of events happens.

This is what the default scripts look like:

```yaml
wyoming:
  script_example:
    event:
      run-satellite: Detect()
      pause-satellite: Stop()
      voice-stopped: Pause()
      audio-stop: PlayAudio() && WriteEvent("played") && Detect()
      error: Detect()
      internal-run: WriteEvent("run-pipeline", '{"start_stage":"wake","end_stage":"tts"}') && Stream()
      internal-detection: WriteEvent("run-pipeline", '{"start_stage":"asr","end_stage":"tts"}') && Stream()
```

Supported functions and variables:

- `Detect()` - start the VAD and WAKE word detection process
- `Stream()` - start transmission of audio data to the client (Home Assistant)
- `Stop()` - stop and disconnect stream without disconnecting client (Home Assistant)
- `Pause()` - temporary pause of audio transfer, without disconnecting the stream
- `PlayAudio()` - playing the last audio that was sent from client (Home Assistant)
- `WriteEvent(type, data)` - send event to client (Home Assistant)
- `Sleep(duration)` - temporary script pause (ex. `Sleep('1.5s')`)
- `PlayFile(path)` - play audio from `wav` file
- `Type` - type (name) of event
- `Data` - event data in JSON format (ex. `{"text":"how are you"}`)
- also available other functions from [expr](https://github.com/AlexxIT/go2rtc/blob/master/internal/expr/README.md) module (ex. `fetch`)

If you write a script for an event - the default action is no longer executed. You need to repeat the necessary steps yourself.

In addition to the standard events, there are two additional events:

- `internal-run` - called after `Detect()` when VAD detected, but WAKE service unavailable
- `internal-detection` - called after `Detect()` when WAKE word detected

**Example 1.** You want to play a sound file when a wake word detected (only `wav` supported):

- `PlayFile` and `PlayAudio` functions are executed synchronously, the following steps will be executed only after they are completed

```yaml
wyoming:
  script_example:
    event:
      internal-detection: PlayFile('/media/beep.wav') && WriteEvent("run-pipeline", '{"start_stage":"asr","end_stage":"tts"}') && Stream()
```

**Example 2.** You want to play TTS on a Home Assistant media player:

Each event has a `Type` and `Data` in JSON format. You can use their values in scripts.

- in the `synthesize` step, we get the value of the `text` and call the HA REST API
- in the `audio-stop` step we get the duration of the TTS in seconds, wait for this time and start the pipeline again

```yaml
wyoming:
  script_example:
    event:
      synthesize: |
        let text = fromJSON(Data).text;
        let token = 'eyJhbGci...';
        fetch('http://localhost:8123/api/services/tts/speak', {
          method: 'POST',
          headers: {'Authorization': 'Bearer '+token,'Content-Type': 'application/json'},
          body: toJSON({
            entity_id: 'tts.google_translate_com',
            media_player_entity_id: 'media_player.google_nest',
            message: text,
            language: 'en',
          }),
        }).ok
      audio-stop: |
        let timestamp = fromJSON(Data).timestamp;
        let delay = string(timestamp)+'s';
        Sleep(delay) && WriteEvent("played") && Detect()
```

## Config examples

Satellite on Windows server using FFmpeg and FFplay.

```yaml
streams:
  satellite_win:
    - exec:ffmpeg -hide_banner -f dshow -i "audio=Microphone (High Definition Audio Device)" -c pcm_s16le -ar 16000 -ac 1 -f wav -
    - exec:ffplay -hide_banner -nodisp -probesize 32 -f s16le -ar 22050 -#backchannel=1#audio=s16le/22050

wyoming:
  satellite_win:
    listen: :10700
    name: "Windows Satellite"
    wake_uri: tcp://192.168.1.23:10400
    vad_threshold: 1
```

Satellite on Dahua camera with two-way audio support.

```yaml
streams:
  dahua_camera:
    - rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=1&unicast=true&proto=Onvif

wyoming:
  dahua_camera:
    listen: :10700
    name: "Dahua Satellite"
    wake_uri: tcp://192.168.1.23:10400
    vad_threshold: 1
```

Satellite on external wyoming Microphone and Sound.

```yaml
streams:
  wyoming_external:
     - wyoming://192.168.1.23:10600                # wyoming-mic-external
     - wyoming://192.168.1.23:10601?backchannel=1  # wyoming-snd-external

wyoming:
   wyoming_external:
    listen: :10700
    name: "Wyoming Satellite"
    wake_uri: tcp://192.168.1.23:10400
    vad_threshold: 1
```

## Wyoming External Microphone and Sound

Advanced users, who want to enjoy the [Wyoming Satellite](https://github.com/rhasspy/wyoming-satellite) project, can use go2rtc as a [Wyoming External Microphone](https://github.com/rhasspy/wyoming-mic-external) or [Wyoming External Sound](https://github.com/rhasspy/wyoming-snd-external).

**go2rtc.yaml**

```yaml
streams:
  wyoming_mic_external:
    - exec:ffmpeg -hide_banner -f dshow -i "audio=Microphone (High Definition Audio Device)" -c pcm_s16le -ar 16000 -ac 1 -f wav -
  wyoming_snd_external:
    - exec:ffplay -hide_banner -nodisp -probesize 32 -f s16le -ar 22050 -#backchannel=1#audio=s16le/22050

wyoming:
  wyoming_mic_external:
    listen: :10600
    mode: mic
  wyoming_snd_external:
    listen: :10601
    mode: snd
```

**docker-compose.yml**

```yaml
version: "3.8"
services:
  satellite:
    build: wyoming-satellite  # https://github.com/rhasspy/wyoming-satellite
    ports:
      - "10700:10700"
    command:
      - "--name"
      - "my satellite"
      - "--mic-uri"
      - "tcp://192.168.1.23:10600"
      - "--snd-uri"
      - "tcp://192.168.1.23:10601"
      - "--debug"
```

## Wyoming External Source

**go2rtc.yaml**

```yaml
streams:
  wyoming_external:
    - wyoming://192.168.1.23:10600
    - wyoming://192.168.1.23:10601?backchannel=1
```

**docker-compose.yml**

```yaml
version: "3.8"
services:
   microphone:
      build: wyoming-mic-external  # https://github.com/rhasspy/wyoming-mic-external
      ports:
         - "10600:10600"
      devices:
         - /dev/snd:/dev/snd
      group_add:
         - audio
      command:
         - "--device"
         - "sysdefault"
         - "--debug"
   playback:
      build: wyoming-snd-external  # https://github.com/rhasspy/wyoming-snd-external
      ports:
         - "10601:10601"
      devices:
         - /dev/snd:/dev/snd
      group_add:
         - audio
      command:
         - "--device"
         - "sysdefault"
         - "--debug"
```

## Debug

```yaml
log:
  wyoming: trace
```
