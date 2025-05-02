## Backchannel

- You can check audio card names in the **Go2rtc > WebUI > Add**
- You can specify multiple backchannel lines with different codecs

```yaml
sources:
  two_way_audio_win:
    - exec:ffmpeg -hide_banner -f dshow -i "audio=Microphone (High Definition Audio Device)" -c pcm_s16le -ar 16000 -ac 1 -f wav -
    - exec:ffplay -nodisp -probesize 32 -f s16le -ar 16000 -#backchannel=1#audio=s16le/16000
    - exec:ffplay -nodisp -probesize 32 -f alaw -ar 8000 -#backchannel=1#audio=alaw/8000
```
