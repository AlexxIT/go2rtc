# Expr

[Expr](https://github.com/antonmedv/expr) - expression language and expression evaluation for Go.

- [language definition](https://expr.medv.io/docs/Language-Definition) - takes best from JS, Python, Jinja2 syntax
- your expression should return a link of any supported source
- expression supports multiple operation, but:
  - all operations must be separated by a semicolon
  - all operations, except the last one, must declare a new variable (`let s = "abc";`)
  - the last operation should return a string
- go2rtc supports additional functions:
  - `fetch` - JS-like HTTP requests
  - `match` - JS-like RegExp queries

## Examples

**Two way audio for Dahua VTO**

```yaml
streams:
  dahua_vto: |
    expr: let host = "admin:password@192.168.1.123";
    fetch("http://"+host+"/cgi-bin/configManager.cgi?action=setConfig&Encode[0].MainFormat[0].Audio.Compression=G.711A&Encode[0].MainFormat[0].Audio.Frequency=8000").ok
        ? "rtsp://"+host+"/cam/realmonitor?channel=1&subtype=0&unicast=true&proto=Onvif" : ""
```

**dom.ru**

You can get credentials via:

- https://github.com/alexmorbo/domru (file `/share/domru/accounts`)
- https://github.com/ad/domru

```yaml
streams:
  dom_ru: |
    expr: let camera = "99999999"; let token = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"; let operator = 99;
    fetch("https://myhome.novotelecom.ru/rest/v1/forpost/cameras/"+camera+"/video", {
      headers: {Authorization: "Bearer "+token, Operator: operator}
    }).json().data.URL
```

**Parse HLS files from Apple**

Same example in two languages - python and expr.

```yaml
streams:
  example_python: |
    echo:python -c 'from urllib.request import urlopen; import re

    # url1 = "https://devstreaming-cdn.apple.com/videos/streaming/examples/bipbop_16x9/bipbop_16x9_variant.m3u8"
    html1 = urlopen("https://developer.apple.com/streaming/examples/basic-stream-osx-ios5.html").read().decode("utf-8")
    url1 = re.search(r"https.+?m3u8", html1)[0]

    # url2 = "gear1/prog_index.m3u8"
    html2 = urlopen(url1).read().decode("utf-8")
    url2 = re.search(r"^[a-z0-1/_]+\.m3u8$", html2, flags=re.MULTILINE)[0]

    # url3 = "https://devstreaming-cdn.apple.com/videos/streaming/examples/bipbop_16x9/gear1/prog_index.m3u8"
    url3 = url1[:url1.rindex("/")+1] + url2

    print("ffmpeg:" + url3 + "#video=copy")'

  example_expr: |
    expr:

    let html1 = fetch("https://developer.apple.com/streaming/examples/basic-stream-osx-ios5.html").text;
    let url1 = match(html1, "https.+?m3u8")[0];

    let html2 = fetch(url1).text;
    let url2 = match(html2, "^[a-z0-1/_]+\\.m3u8$", "m")[0];

    let url3 = url1[:lastIndexOf(url1, "/")+1] + url2;

    "ffmpeg:" + url3 + "#video=copy"
```

## Comparsion

| expr                         | python                     | js                             |
|------------------------------|----------------------------|--------------------------------|
| let x = 1;                   | x = 1                      | let x = 1                      |
| {a: 1, b: 2}                 | {"a": 1, "b": 2}           | {a: 1, b: 2}                   |
| let r = fetch(url, {method}) | r = request(method, url)   | r = await fetch(url, {method}) |
| r.ok                         | r.ok                       | r.ok                           |
| r.status                     | r.status_code              | r.status                       |
| r.text                       | r.text                     | await r.text()                 |
| r.json()                     | r.json()                   | await r.json()                 |
| r.headers                    | r.headers                  | r.headers                      |
| let m = match(text, "abc")   | m = re.search("abc", text) | let m = text.match(/abc/)      |
