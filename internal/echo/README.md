# Echo

Some sources may have a dynamic link. And you will need to get it using a Bash or Python script. Your script should echo a link to the source. RTSP, FFmpeg or any of the supported sources.

**Docker** and **Home Assistant add-on** users have preinstalled `python3`, `curl`, `jq`.

## Configuration

```yaml
streams:
  apple_hls: echo:python3 hls.py https://developer.apple.com/streaming/examples/basic-stream-osx-ios5.html
```

## Install python libraries

**Docker** and **Hass Add-on** users have preinstalled `python3` without any additional libraries, like [requests](https://requests.readthedocs.io/) or others. If you need some additional libraries - you need to install them to folder with your script:

1. Install [SSH & Web Terminal](https://github.com/hassio-addons/addon-ssh)
2. Goto Add-on Web UI
3. Install library: `pip install requests -t /config/echo`
4. Add your script to `/config/echo/myscript.py`
5. Use your script as source `echo:python3 /config/echo/myscript.py`

## Example: Apple HLS

```yaml
streams:
  apple_hls: echo:python3 hls.py https://developer.apple.com/streaming/examples/basic-stream-osx-ios5.html
```

**hls.py**

```python
import re
import sys
from urllib.parse import urljoin
from urllib.request import urlopen

html = urlopen(sys.argv[1]).read().decode("utf-8")
url = re.search(r"https.+?m3u8", html)[0]

html = urlopen(url).read().decode("utf-8")
m = re.search(r"^[a-z0-1/_]+\.m3u8$", html, flags=re.MULTILINE)
url = urljoin(url, m[0])

# ffmpeg:https://devstreaming-cdn.apple.com/videos/streaming/examples/bipbop_16x9/gear1/prog_index.m3u8#video=copy
print("ffmpeg:" + url + "#video=copy")
```
