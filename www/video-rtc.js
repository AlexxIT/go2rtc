/**
 * Common function for processing MSE and MSE2 data.
 * @param {MediaSource} ms
 * @returns {Function}
 */
function MediaSourceHandler(ms) {
    /** @type {SourceBuffer} */
    let sb;

    const bufCap = 2 * 1024 * 1024;
    const buf = new Uint8Array(bufCap);
    let bufLen = 0;

    return ev => {
        if (typeof ev.data === "string") {
            const msg = JSON.parse(ev.data);
            if (msg.type === "mse") {
                if (!MediaSource.isTypeSupported(msg.value)) {
                    console.warn("Not supported: " + msg.value)
                    return;
                }

                sb = ms.addSourceBuffer(msg.value);
                sb.mode = "segments"; // segments or sequence
                sb.addEventListener("updateend", () => {
                    if (sb.updating) return;
                    if (bufLen > 0) {
                        try {
                            sb.appendBuffer(buf.slice(0, bufLen));
                        } catch (e) {
                            console.debug(e);
                        }
                        bufLen = 0;
                    } else if (sb.buffered.length) {
                        const end = sb.buffered.end(sb.buffered.length - 1) - 5;
                        const start = sb.buffered.start(0);
                        if (end > start) {
                            sb.remove(start, end);
                            ms.setLiveSeekableRange(end, end + 5);
                        }
                        // console.debug("VideoRTC.buffered", start, end);
                    }
                });
            }
        } else if (sb.updating || bufLen > 0) {
            const b = new Uint8Array(ev.data);
            buf.set(b, bufLen);
            bufLen += b.byteLength;
            // console.debug("VideoRTC.buffer", b.byteLength, bufLen);
        } else {
            try {
                sb.appendBuffer(ev.data);
            } catch (e) {
                // console.debug(e);
            }
        }
    }
}

/**
 * Dedicated Worker Handler for MSE2 https://chromestatus.com/feature/5177263249162240
 */
if (typeof importScripts == "function") {
    // protect below code (class VideoRTC) from fail inside Worker
    HTMLElement = Object;
    customElements = {define: Function()};

    const ms = new MediaSource();
    ms.addEventListener("sourceopen", ev => {
        postMessage({type: ev.type});
    }, {once: true});

    onmessage = MediaSourceHandler(ms);

    postMessage({type: "handle", value: ms.handle}, [ms.handle]);
}

/**
 * Video player for MSE and WebRTC connections.
 *
 * All modern web technologies are supported in almost any browser except Apple Safari.
 *
 * Support:
 * - RTCPeerConnection for Safari iOS 11.0+
 * - IntersectionObserver for Safari iOS 12.2+
 * - MediaSource in Workers for Chrome 108+
 *
 * Doesn't support:
 * - MediaSource for Safari iOS all
 * - Customized built-in elements (extends HTMLVideoElement) because all Safari
 * - Public class fields because old Safari (before 14.0)
 */
class VideoRTC extends HTMLElement {
    constructor() {
        super();

        this.DISCONNECT_TIMEOUT = 5000;
        this.RECONNECT_TIMEOUT = 30000;

        this.CODECS = [
            "avc1.640029",      // H.264 high 4.1 (Chromecast 1st and 2nd Gen)
            "avc1.64002A",      // H.264 high 4.2 (Chromecast 3rd Gen)
            "hvc1.1.6.L153.B0", // H.265 main 5.1 (Chromecast Ultra)
            "mp4a.40.2",        // AAC LC
            "mp4a.40.5",        // AAC HE
            "mp4a.69",          // MP3
            "mp4a.6B",          // MP3
        ];

        /**
         * Supported modes.
         * @type {string}
         */
        this.modes = "webrtc,mse2,mp4";

        /**
         * Run stream when not displayed on the screen. Default `false`.
         * @type {boolean}
         */
        this.background = false;

        /**
         * Run stream only when player in the viewport. Stop when user scroll out player.
         * Value is percentage of visibility from `0` (not visible) to `1` (full visible).
         * Default `0` - disable;
         * @type {number}
         */
        this.intersectionThreshold = 0;

        /**
         * Run stream only when browser page on the screen. Stop when user change browser
         * tab or minimise browser windows.
         * @type {boolean}
         */
        this.visibilityCheck = true;

        /**
         * @type {HTMLVideoElement}
         */
        this.video = null;

        /**
         * @type {WebSocket}
         */
        this.ws = null;

        /**
         * Internal WebSocket connection state. Values: CONNECTING, OPEN, CLOSED
         * @type {number}
         */
        this.wsState = WebSocket.CLOSED;

        /**
         * Internal WebSocket URL.
         * @type {string|URL}
         */
        this.url = "";

        /**
         * @type {RTCPeerConnection}
         */
        this.pc = null;

        /**
         * @type {number}
         */
        this.pcState = WebSocket.CLOSED;

        this.pcConfig = {iceServers: [{urls: "stun:stun.l.google.com:19302"}]};

        /**
         * Internal disconnect TimeoutID.
         * @type {number}
         */
        this.disconnectTimeout = 0;

        /**
         * Internal reconnect TimeoutID.
         * @type {number}
         */
        this.reconnectTimeout = 0;
    }

    /** public properties **/

    /**
     * Set video source (WebSocket URL). Support relative path.
     * @param {string|URL} value
     */
    set src(value) {
        if (typeof value === "string") {
            if (value.startsWith("/")) {
                value = "ws" + location.origin.substring(4) + value;
            } else if (value.startsWith("http")) {
                value = "ws" + value.substring(4);
            }
        }

        this.url = value;

        if (this.isConnected) this.connectedCallback();
    }

    /**
     * Play video. Support automute when autoplay blocked.
     * https://developer.chrome.com/blog/autoplay/
     */
    play() {
        this.video.play().catch(er => {
            if (er.name === "NotAllowedError" && !this.video.muted) {
                this.video.muted = true;
                this.video.play().catch(() => console.debug);
            }
        });
    }

    /**
     * Send message to server via WebSocket
     * @param {Object} value
     */
    send(value) {
        if (this.ws) this.ws.send(JSON.stringify(value));
    }

    get closed() {
        return this.wsState === WebSocket.CLOSED && this.pcState === WebSocket.CLOSED;
    }

    codecs(type) {
        const test = type === "mse"
            ? codec => MediaSource.isTypeSupported(`video/mp4; codecs="${codec}"`)
            : codec => this.video.canPlayType(`video/mp4; codecs="${codec}"`);
        return this.CODECS.filter(test).join();
    }

    /**
     * `CustomElement`. Invoked each time the custom element is appended into a
     * document-connected element.
     */
    connectedCallback() {
        console.debug("VideoRTC.connectedCallback", this.wsState, this.pcState);

        if (this.disconnectTimeout) {
            clearTimeout(this.disconnectTimeout);
            this.disconnectTimeout = 0;
        }

        // because video autopause on disconnected from DOM
        if (this.video) {
            const seek = this.video.seekable;
            if (seek.length > 0) {
                this.video.currentTime = seek.end(seek.length - 1);
                this.play();
            }
        }

        if (!this.url || !this.closed) return;

        // CLOSED => CONNECTING
        this.wsState = WebSocket.CONNECTING;

        this.internalInit();
        this.internalWS();
    }

    /**
     * `CustomElement`. Invoked each time the custom element is disconnected from the
     * document's DOM.
     */
    disconnectedCallback() {
        console.debug("VideoRTC.disconnectedCallback", this.wsState, this.pcState);

        if (this.background || this.disconnectTimeout || this.closed) return;

        this.disconnectTimeout = setTimeout(() => {
            if (this.reconnectTimeout) {
                clearTimeout(this.reconnectTimeout);
                this.reconnectTimeout = 0;
            }

            this.disconnectTimeout = 0;

            this.wsState = WebSocket.CLOSED;
            if (this.ws) {
                this.ws.close();
                this.ws = null;
            }

            this.pcState = WebSocket.CLOSED;
            if (this.pc) {
                this.pc.close();
                this.pc = null;
            }
        }, this.DISCONNECT_TIMEOUT);
    }

    internalInit() {
        if (this.childElementCount) return;

        this.video = document.createElement("video");
        this.video.controls = true;
        this.video.playsInline = true;
        this.video.preload = "auto";

        this.appendChild(this.video);

        // don't know if styles like this is good idea
        this.style.display = "block";
        this.style.position = "relative";
        this.style.width = "100%";
        this.style.height = "100%";

        // video position absolute important for second video child
        this.video.style.position = "absolute";
        this.video.style.width = "100%";
        this.video.style.height = "100%"

        if (this.background) return;

        if ("hidden" in document && this.visibilityCheck) {
            document.addEventListener("visibilitychange", () => {
                if (document.hidden) {
                    this.disconnectedCallback();
                } else if (this.isConnected) {
                    this.connectedCallback();
                }
            })
        }

        if ("IntersectionObserver" in window && this.intersectionThreshold) {
            const observer = new IntersectionObserver(entries => {
                entries.forEach(entry => {
                    if (!entry.isIntersecting) {
                        this.disconnectedCallback();
                    } else if (this.isConnected) {
                        this.connectedCallback();
                    }
                });
            }, {threshold: this.intersectionThreshold});
            observer.observe(this);
        }
    }

    internalWS() {
        if (this.wsState !== WebSocket.CONNECTING) return;
        if (this.ws) throw "connect with non null WebSocket";

        const ts = Date.now();

        this.ws = new WebSocket(this.url);

        this.ws.addEventListener("open", () => {
            console.debug("VideoRTC.ws.open", this.wsState);

            // CONNECTING => OPEN
            this.wsState = WebSocket.OPEN;

            if (this.modes.indexOf("mse") >= 0 && "MediaSource" in window) { // iPhone
                if (this.modes.indexOf("mse2") >= 0 && MediaSource.canConstructInDedicatedWorker) {
                    this.internalMSE2();
                } else {
                    this.internalMSE();
                }
            } else if (this.modes.indexOf("mp4") >= 0) {
                this.internalMP4();
            }

            if (this.modes.indexOf("webrtc") >= 0 && "RTCPeerConnection" in window) { // macOS Desktop app
                this.internalRTC();
            }

            if (this.modes.indexOf("mjpeg") >= 0) {
                this.internalMJPEG();
            }
        });

        this.ws.addEventListener("close", () => {
            console.debug("VideoRTC.ws.close", this.wsState);

            if (this.wsState === WebSocket.CLOSED) return;

            // CONNECTING, OPEN => CONNECTING
            this.wsState = WebSocket.CONNECTING;
            this.ws = null;

            // reconnect no more than once every X seconds
            const delay = Math.max(this.RECONNECT_TIMEOUT - (Date.now() - ts), 0);

            this.reconnectTimeout = setTimeout(() => {
                this.reconnectTimeout = 0;
                this.internalWS();
            }, delay);
        });
    }

    internalMSE() {
        console.debug("VideoRTC.internalMSE");

        const ms = new MediaSource();
        ms.addEventListener("sourceopen", () => {
            console.debug("VideoRTC.ms.sourceopen");
            URL.revokeObjectURL(this.video.src);
            this.send({type: "mse", value: this.codecs("mse")});
        }, {once: true});

        this.video.src = URL.createObjectURL(ms);
        this.video.srcObject = null;
        this.play();

        this.ws.binaryType = "arraybuffer";
        this.ws.addEventListener("message", MediaSourceHandler(ms));
    }

    internalMSE2() {
        console.debug("VideoRTC.internalMSE2");

        const worker = new Worker("video-rtc.js");
        worker.addEventListener("message", ev => {
            if (ev.data.type === "handle") {
                this.video.srcObject = ev.data.value;
                this.play();
            } else if (ev.data.type === "sourceopen") {
                this.send({type: "mse", value: this.codecs("mse")});
            }
        });

        this.ws.binaryType = "arraybuffer";
        this.ws.addEventListener("message", ev => {
            if (typeof ev.data === "string") {
                worker.postMessage(ev.data);
            } else {
                worker.postMessage(ev.data, [ev.data]);
            }
        });

        this.ws.addEventListener("close", () => {
            worker.terminate();
        });
    }

    internalRTC() {
        console.debug("VideoRTC.internalRTC");

        const pc = new RTCPeerConnection(this.pcConfig);

        let mseCodecs = "";

        /** @type {HTMLVideoElement} */
        const video2 = document.createElement("video");
        video2.addEventListener("loadeddata", () => {
            console.debug("VideoRTC.video.loadeddata", video2.readyState, pc.connectionState);

            if (pc.connectionState === "connected" || pc.connectionState === "connecting") {
                // Video+Audio > Video, H265 > H264, Video > Audio, WebRTC > MSE
                let rtcPriority = 0, msePriority = 0;

                /** @type {MediaStream} */
                const rtc = video2.srcObject;
                if (rtc.getVideoTracks().length > 0) rtcPriority += 0x220;
                if (rtc.getAudioTracks().length > 0) rtcPriority += 0x102;

                if (mseCodecs.indexOf("hvc1.") >= 0) msePriority += 0x230;
                if (mseCodecs.indexOf("avc1.") >= 0) msePriority += 0x210;
                if (mseCodecs.indexOf("mp4a.") >= 0) msePriority += 0x101;

                if (rtcPriority >= msePriority) {
                    console.debug("VideoRTC.select RTC mode", rtcPriority, msePriority);

                    this.video.controls = true;
                    this.video.srcObject = rtc;
                    this.play();

                    this.pcState = WebSocket.OPEN;

                    this.wsState = WebSocket.CLOSED;
                    this.ws.close();
                    this.ws = null;
                } else {
                    console.debug("VideoRTC.select MSE mode", rtcPriority, msePriority);

                    pc.close();

                    this.pcState = WebSocket.CLOSED;
                    this.pc = null;
                }
            }

            video2.srcObject = null;
        }, {once: true});

        pc.addEventListener("icecandidate", ev => {
            const candidate = ev.candidate ? ev.candidate.toJSON().candidate : "";
            this.send({type: "webrtc/candidate", value: candidate});
        });

        pc.addEventListener("track", ev => {
            console.debug("VideoRTC.pc.track", ev.streams.length);

            // when stream already init
            if (video2.srcObject !== null) return;

            // when audio track not exist in Chrome
            if (ev.streams.length === 0) return;

            // when audio track not exist in Firefox
            if (ev.streams[0].id[0] === '{') return;

            video2.srcObject = ev.streams[0];
        });

        pc.addEventListener("connectionstatechange", () => {
            console.debug("VideoRTC.pc.connectionstatechange", this.pc.connectionState);

            if (pc.connectionState === "failed" || pc.connectionState === "disconnected") {
                pc.close(); // stop next events

                this.pcState = WebSocket.CLOSED;
                this.pc = null;

                if (this.wsState === WebSocket.CLOSED && this.isConnected) {
                    this.connectedCallback();
                }
            }
        });

        this.ws.addEventListener("message", ev => {
            if (typeof ev.data !== "string") return;

            const msg = JSON.parse(ev.data);
            switch (msg.type) {
                case "webrtc/candidate":
                    pc.addIceCandidate({candidate: msg.value, sdpMid: ""}).catch(() => console.debug);
                    break;
                case "webrtc/answer":
                    pc.setRemoteDescription({type: "answer", sdp: msg.value}).catch(() => console.debug);
                    break;
                case "mse":
                    mseCodecs = msg.value;
                    break;
            }
        });

        // Safari doesn't support "offerToReceiveVideo"
        pc.addTransceiver("video", {direction: "recvonly"});
        pc.addTransceiver("audio", {direction: "recvonly"});

        pc.createOffer().then(offer => {
            pc.setLocalDescription(offer).then(() => {
                this.send({type: "webrtc/offer", value: offer.sdp});
            });
        });

        this.pcState = WebSocket.CONNECTING;
        this.pc = pc;
    }

    internalMJPEG() {
        console.debug("VideoRTC.internalMJPEG");

        const reader = new FileReader();
        reader.addEventListener("load", () => {
            this.video.poster = reader.result;
        });

        this.ws.binaryType = "blob";
        this.ws.addEventListener("message", ev => {
            if (typeof ev.data !== "string") {
                try {
                    reader.readAsDataURL(ev.data);
                } catch (e) {
                    console.debug(e);
                }
            }
        });

        this.send({type: "mjpeg"});
        this.video.controls = false;
    }

    internalMP4() {
        console.debug("VideoRTC.internalMP4");

        /** @type {HTMLVideoElement} */
        let video2;

        /** @type {number} */
        let i;

        const reader = new FileReader();
        reader.addEventListener("load", () => {
            if (video2) {
                this.removeChild(this.video);
                this.video.src = "";
                this.video = video2;
            } else {
                // get position only once on first packet
                i = reader.result.indexOf(";");
                console.debug("VideoRTC.file", reader.result.substring(0, i));
            }

            video2 = this.video.cloneNode();
            this.appendChild(video2);

            video2.src = "data:video/mp4" + reader.result.substring(i);
            video2.play().catch(() => console.log);
        });

        this.ws.binaryType = "blob";
        this.ws.addEventListener("message", ev => {
            if (typeof ev.data !== "string") {
                try {
                    reader.readAsDataURL(ev.data);
                } catch (e) {
                    console.debug(e);
                }
            }
        });

        this.ws.addEventListener("close", () => {
            if (video2) {
                this.removeChild(video2);
                video2.src = "";
            }
        });

        this.send({type: "mp4", value: this.codecs("mp4")});
        this.video.controls = false;
    }
}

customElements.define("video-rtc", VideoRTC);
