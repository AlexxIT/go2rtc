# HTML5

**1. Autoplay video tag**

[Video auto play is not working](https://stackoverflow.com/questions/17994666/video-auto-play-is-not-working-in-safari-and-chrome-desktop-browser)

> Recently many browsers can only autoplay the videos with sound off, so you'll need to add muted attribute to the video tag too

```html

<video id="video" autoplay controls playsinline muted></video>
```

**2. [Safari] pc.createOffer**

Don't work in Desktop Safari:

```js
pc.createOffer({offerToReceiveAudio: true, offerToReceiveVideo: true})
```

Should be replaced with:

```js
pc.addTransceiver('video', {direction: 'recvonly'});
pc.addTransceiver('audio', {direction: 'recvonly'});
pc.createOffer();
```

**3. pc.ontrack**

TODO

```js
pc.ontrack = ev => {
    const video = document.getElementById('video');

    // when audio track not exist in Chrome
    if (ev.streams.length === 0) return;

    // when audio track not exist in Firefox
    if (ev.streams[0].id[0] === '{') return;

    // when stream already init
    if (video.srcObject !== null) return;

    video.srcObject = ev.streams[0];
}
```

## Chromecast 1

2023-02-02. Error:

```
InvalidStateError: Failed to execute 'addTransceiver' on 'RTCPeerConnection': This operation is only supported in 'unified-plan'. 'unified-plan' will become the default behavior in the future, but it is currently experimental. To try it out, construct the RTCPeerConnection with sdpSemantics:'unified-plan' present in the RTCConfiguration argument.
```

User-Agent: `Mozilla/5.0 (X11; Linux armv7l) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/70.0.3538.47 Safari/537.36 CrKey/1.36.159268`

https://webrtc.org/getting-started/unified-plan-transition-guide?hl=en

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
