import {VideoRenderer} from './video-renderer.js';

/**
 * WebCodecsPlayer — handles video/audio decoding, controls, and rendering.
 *
 * Usage:
 *   const player = new WebCodecsPlayer(parentElement, {cascade, media});
 *   const result = player.init(info);      // {error?: string}
 *   player.feed(data);                     // binary frame
 *   player.unmute();                       // start audio (user gesture)
 *   player.destroy();
 */

const HEADER_SIZE = 9;

// SVG icon paths (24x24 viewBox)
const ICONS = {
    play: 'M8 5v14l11-7z',
    pause: 'M6 19h4V5H6v14zm8-14v14h4V5h-4z',
    volume: 'M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02zM14 3.23v2.06c2.89.86 5 3.54 5 6.71s-2.11 5.85-5 6.71v2.06c4.01-.91 7-4.49 7-8.77s-2.99-7.86-7-8.77z',
    muted: 'M16.5 12c0-1.77-1.02-3.29-2.5-4.03v2.21l2.45 2.45c.03-.2.05-.41.05-.63zm2.5 0c0 .94-.2 1.82-.54 2.64l1.51 1.51C20.63 14.91 21 13.5 21 12c0-4.28-2.99-7.86-7-8.77v2.06c2.89.86 5 3.54 5 6.71zM4.27 3L3 4.27 7.73 9H3v6h4l5 5v-6.73l4.25 4.25c-.67.52-1.42.93-2.25 1.18v2.06c1.38-.31 2.63-.95 3.69-1.81L19.73 21 21 19.73l-9-9L4.27 3zM12 4L9.91 6.09 12 8.18V4z',
    fs: 'M7 14H5v5h5v-2H7v-3zm-2-4h2V7h3V5H5v5zm12 7h-3v2h5v-5h-2v3zM14 5v2h3v3h2V5h-5z',
    fsExit: 'M5 16h3v3h2v-5H5v2zm3-8H5v2h5V5H8v3zm6 11h2v-3h3v-2h-5v5zm2-11V5h-2v5h5V8h-3z',
};

function svgIcon(path) {
    return `<svg viewBox="0 0 24 24" style="width:20px;height:20px;fill:#fff"><path d="${path}"/></svg>`;
}

export class WebCodecsPlayer {
    /**
     * @param {HTMLElement} parent — element to insert container into
     * @param {object} [options]
     * @param {string} [options.cascade='webgpu,webgl,2d'] — renderer cascade
     * @param {string} [options.media='video,audio'] — requested media types
     */
    constructor(parent, options = {}) {
        this._parent = parent;
        this._media = options.media || 'video,audio';
        this._paused = false;
        this._muted = true;

        // Decoders
        this._videoDecoder = null;
        this._audioDecoder = null;
        this._audioCtx = null;
        this._gainNode = null;
        this._audioInfo = null;
        this._audioStarted = false;

        // Build DOM
        this._container = this._createContainer();
        this._renderer = new VideoRenderer(this._container, {
            cascade: options.cascade || 'webgpu,webgl,2d',
            canvasStyle: 'display:block;max-width:100%;max-height:100%',
        });
    }

    /**
     * Initialize decoders with server info.
     * @param {{video?: {codec: string}, audio?: {codec: string, sampleRate: number, channels: number}}} info
     * @returns {Promise<{error?: string}>}
     */
    async init(info) {
        VideoRenderer.log('init:', info.video ? 'video=' + info.video.codec : 'no video',
            info.audio ? 'audio=' + info.audio.codec + ' ' + info.audio.sampleRate + 'Hz' : 'no audio');

        if (info.video) {
            const config = {codec: info.video.codec, optimizeForLatency: true};
            try {
                const support = await VideoDecoder.isConfigSupported(config);
                if (!support.supported) {
                    VideoRenderer.log('VideoDecoder: codec not supported:', info.video.codec);
                    return {error: 'video codec not supported: ' + info.video.codec};
                }
                this._videoDecoder = new VideoDecoder({
                    output: frame => {
                        this._renderer.draw(frame);
                        frame.close();
                    },
                    error: err => VideoRenderer.log('VideoDecoder error:', err),
                });
                this._videoDecoder.configure(support.config);
                VideoRenderer.log('VideoDecoder: configured', info.video.codec);
            } catch (err) {
                VideoRenderer.log('VideoDecoder: config check failed:', err.message || err);
                return {error: err.message || String(err)};
            }
        }

        if (info.audio && this._media.includes('audio')) {
            this._audioInfo = info.audio;
            this._audioStarted = false;
        } else {
            this._hideAudioControls();
        }

        this._parent.insertBefore(this._container, this._parent.firstChild);
        return {};
    }

    /**
     * Feed a binary frame from WebSocket.
     * @param {ArrayBuffer} data
     */
    feed(data) {
        if (this._paused || data.byteLength < HEADER_SIZE) return;

        const view = new DataView(data);
        const flags = view.getUint8(0);
        const isVideo = (flags & 0x80) !== 0;
        const isKeyframe = (flags & 0x40) !== 0;
        const timestamp = Number(view.getBigUint64(1));
        const payload = new Uint8Array(data, HEADER_SIZE);

        if (isVideo && this._videoDecoder?.state === 'configured') {
            this._videoDecoder.decode(new EncodedVideoChunk({
                type: isKeyframe ? 'key' : 'delta',
                timestamp,
                data: payload,
            }));
        } else if (!isVideo && this._audioDecoder?.state === 'configured') {
            this._audioDecoder.decode(new EncodedAudioChunk({
                type: 'key',
                timestamp,
                data: payload,
            }));
        }
    }

    /** Start audio playback. Call from a user gesture (click handler). */
    unmute() {
        this._muted = false;
        this._startAudio();
        this._updateVolume();
    }

    /** Stop audio playback. */
    mute() {
        this._muted = true;
        this._updateVolume();
    }

    /** @returns {boolean} */
    get paused() { return this._paused; }

    /** @returns {boolean} */
    get muted() { return this._muted; }

    /** @returns {HTMLElement} */
    get container() { return this._container; }

    destroy() {
        if (this._videoDecoder) {
            try { this._videoDecoder.close(); } catch {}
            this._videoDecoder = null;
        }
        if (this._audioDecoder) {
            try { this._audioDecoder.close(); } catch {}
            this._audioDecoder = null;
        }
        if (this._gainNode) this._gainNode = null;
        if (this._audioCtx) {
            try { this._audioCtx.close(); } catch {}
            this._audioCtx = null;
        }
        this._audioInfo = null;
        this._audioStarted = false;
        if (this._renderer) {
            this._renderer.destroy();
            this._renderer = null;
        }
        if (this._container?.parentElement) {
            this._container.remove();
        }
    }

    _createContainer() {
        const container = document.createElement('div');
        container.style.cssText = 'position:relative;width:100%;height:100%;background:#000;' +
            'display:flex;align-items:center;justify-content:center;overflow:hidden';

        const controls = document.createElement('div');
        controls.style.cssText = 'position:absolute;bottom:0;left:0;right:0;display:flex;' +
            'align-items:center;gap:4px;padding:4px 8px;background:rgba(23,23,23,.85);' +
            'opacity:0;transition:opacity .3s;user-select:none;z-index:1;height:36px;box-sizing:border-box';
        container.addEventListener('mouseenter', () => { controls.style.opacity = '1'; });
        container.addEventListener('mouseleave', () => { controls.style.opacity = '0'; });
        container.addEventListener('touchstart', ev => {
            if (ev.target === container || ev.target.tagName === 'CANVAS') {
                controls.style.opacity = controls.style.opacity === '1' ? '0' : '1';
            }
        }, {passive: true});

        const btnStyle = 'background:none;border:none;cursor:pointer;padding:4px;display:flex;' +
            'align-items:center;justify-content:center;opacity:.85';

        // Play/Pause
        const btnPlay = document.createElement('button');
        btnPlay.style.cssText = btnStyle;
        btnPlay.innerHTML = svgIcon(ICONS.pause);
        btnPlay.title = 'Pause';
        btnPlay.addEventListener('click', () => this._togglePause());

        // Live label
        const timeLabel = document.createElement('span');
        timeLabel.style.cssText = 'color:#fff;font-size:12px;font-family:Arial,sans-serif;padding:0 4px;min-width:36px';
        timeLabel.textContent = 'LIVE';

        const spacer = document.createElement('div');
        spacer.style.flex = '1';

        // Mute
        const btnMute = document.createElement('button');
        btnMute.style.cssText = btnStyle;
        btnMute.innerHTML = svgIcon(ICONS.muted);
        btnMute.title = 'Unmute';
        btnMute.addEventListener('click', () => {
            this._muted = !this._muted;
            if (!this._muted) this._startAudio();
            this._updateVolume();
        });

        // Volume slider
        const volume = document.createElement('input');
        volume.type = 'range';
        volume.min = '0';
        volume.max = '1';
        volume.step = '0.05';
        volume.value = '1';
        volume.style.cssText = 'width:60px;cursor:pointer;accent-color:#fff;height:4px';
        volume.addEventListener('input', () => {
            this._muted = false;
            this._startAudio();
            this._updateVolume();
        });

        // Fullscreen
        const btnFS = document.createElement('button');
        btnFS.style.cssText = btnStyle;
        btnFS.innerHTML = svgIcon(ICONS.fs);
        btnFS.title = 'Fullscreen';
        btnFS.addEventListener('click', () => {
            if (document.fullscreenElement) {
                document.exitFullscreen();
            } else {
                container.requestFullscreen().catch(() => {});
            }
        });
        document.addEventListener('fullscreenchange', () => {
            const isFS = document.fullscreenElement === container;
            btnFS.innerHTML = svgIcon(isFS ? ICONS.fsExit : ICONS.fs);
            btnFS.title = isFS ? 'Exit fullscreen' : 'Fullscreen';
        });

        controls.append(btnPlay, timeLabel, spacer, btnMute, volume, btnFS);
        container.append(controls);

        // Store refs for updates
        this._btnPlay = btnPlay;
        this._btnMute = btnMute;
        this._volume = volume;

        return container;
    }

    _hideAudioControls() {
        if (this._btnMute) this._btnMute.style.display = 'none';
        if (this._volume) this._volume.style.display = 'none';
    }

    _togglePause() {
        this._paused = !this._paused;
        this._btnPlay.innerHTML = svgIcon(this._paused ? ICONS.play : ICONS.pause);
        this._btnPlay.title = this._paused ? 'Play' : 'Pause';
        if (this._paused && this._audioCtx) this._audioCtx.suspend();
        if (!this._paused && this._audioCtx) {
            this._audioCtx._nextTime = 0;
            this._audioCtx.resume();
        }
    }

    _updateVolume() {
        if (this._gainNode) {
            this._gainNode.gain.value = this._muted ? 0 : parseFloat(this._volume.value);
        }
        if (this._audioCtx?.state === 'suspended') {
            this._audioCtx.resume();
        }
        const isMuted = this._muted || parseFloat(this._volume.value) === 0;
        this._btnMute.innerHTML = svgIcon(isMuted ? ICONS.muted : ICONS.volume);
        this._btnMute.title = isMuted ? 'Unmute' : 'Mute';
    }

    _startAudio() {
        if (this._audioStarted || !this._audioInfo) return;
        this._audioStarted = true;

        const info = this._audioInfo;
        const config = {codec: info.codec, sampleRate: info.sampleRate, numberOfChannels: info.channels};

        AudioDecoder.isConfigSupported(config).then(support => {
            if (!support.supported) {
                VideoRenderer.log('AudioDecoder: codec not supported:', info.codec);
                return;
            }

            const actx = new AudioContext({sampleRate: info.sampleRate});
            this._audioCtx = actx;
            this._gainNode = actx.createGain();
            this._gainNode.connect(actx.destination);

            this._audioDecoder = new AudioDecoder({
                output: data => {
                    if (actx.state === 'closed') { data.close(); return; }
                    const buf = actx.createBuffer(data.numberOfChannels, data.numberOfFrames, data.sampleRate);
                    for (let ch = 0; ch < data.numberOfChannels; ch++) {
                        data.copyTo(buf.getChannelData(ch), {planeIndex: ch, format: 'f32-planar'});
                    }
                    const src = actx.createBufferSource();
                    src.buffer = buf;
                    src.connect(this._gainNode);
                    const now = actx.currentTime;
                    if ((actx._nextTime || 0) < now) {
                        actx._nextTime = now;
                    }
                    src.start(actx._nextTime);
                    actx._nextTime += buf.duration;
                    data.close();
                },
                error: () => { this._audioDecoder = null; },
            });
            this._audioDecoder.configure(support.config);

            VideoRenderer.log('audio started:', info.codec, info.sampleRate + 'Hz', info.channels + 'ch');
            this._updateVolume();
        }).catch(err => {
            VideoRenderer.log('AudioDecoder: config check failed:', err.message || err);
        });
    }
}
