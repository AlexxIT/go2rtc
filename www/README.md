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

## Useful links

- https://www.webrtc-experiment.com/DetectRTC/
- https://divtable.com/table-styler/
- https://www.chromium.org/audio-video/
