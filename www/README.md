# www

This folder contains static HTTP and JS content that is embedded into the application during build. An external developer can use it as a basis for integrating go2rtc into their project or for developing a custom web interface for go2rtc.

## HTTP API

`www/stream.html` - universal viewer with support params in URL:

- multiple streams on page `src=camera1&src=camera2...`
- stream technology autoselection `mode=webrtc,webrtc/tcp,mse,hls,mp4,mjpeg`
- stream technology comparison `src=camera1&mode=webrtc&mode=mse&mode=mp4`
- player width setting in pixels `width=320px` or percents `width=50%`

`www/webrtc.html` - WebRTC viewer with support two way audio and params in URL:

- `media=video+audio` - simple viewer
- `media=video+audio+microphone` - two way audio from camera
- `media=camera+microphone` - stream from browser
- `media=display+speaker` - stream from desktop

## JavaScript API

- You can write your viewer from the scratch
- You can extend the built-in viewer - `www/video-rtc.js`
- Check example - `www/video-stream.js`
- Check example - https://github.com/AlexxIT/WebRTC

`video-rtc.js` features:

- support technologies:
    - WebRTC over UDP or TCP
    - MSE or HLS or MP4 or MJPEG over WebSocket
- automatic selection best technology according on:
    - codecs inside your stream
    - current browser capabilities
    - current network configuration
- automatic stop stream while browser or page not active
- automatic stop stream while player not inside page viewport
- automatic reconnection

Technology selection based on priorities:

1. Video and Audio better than just Video
2. H265 better than H264
3. WebRTC better than MSE, than HLS, than MJPEG

## Browser support

[ECMAScript 2019 (ES10)](https://caniuse.com/?search=es10) supported by [iOS 12](https://en.wikipedia.org/wiki/IOS_12) (iPhone 5S, iPad Air, iPad Mini 2, etc.).

But [ECMAScript 2017 (ES8)](https://caniuse.com/?search=es8) almost fine (`es6 + async`) and recommended for [React+TypeScript](https://github.com/typescript-cheatsheets/react).

## Known problems

- Autoplay doesn't work for WebRTC in Safari [read more](https://developer.apple.com/documentation/webkit/delivering_video_content_for_safari/).

## Useful links

- https://www.webrtc-experiment.com/DetectRTC/
- https://divtable.com/table-styler/
- https://www.chromium.org/audio-video/
- https://web.dev/i18n/en/fast-playback-with-preload/#manual_buffering
- https://developer.mozilla.org/en-US/docs/Web/API/Media_Source_Extensions_API
- https://chromium.googlesource.com/external/w3c/web-platform-tests/+/refs/heads/master/media-source/mediasource-is-type-supported.html
- https://googlechrome.github.io/samples/media/sourcebuffer-changetype.html
- https://chromestatus.com/feature/5100845653819392
- https://developer.apple.com/documentation/webkit/delivering_video_content_for_safari
- https://dirask.com/posts/JavaScript-supported-Audio-Video-MIME-Types-by-MediaRecorder-Chrome-and-Firefox-jERn81
- https://privacycheck.sec.lrz.de/active/fp_cpt/fp_can_play_type.html
