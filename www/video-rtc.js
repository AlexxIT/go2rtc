/**
 * Video player for MSE and WebRTC connections.
 *
 * All modern web technologies are supported in almost any browser except Apple Safari.
 *
 * Support:
 * - RTCPeerConnection for Safari iOS 11.0+
 * - IntersectionObserver for Safari iOS 12.2+
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
            "avc1.640033",      // H.264 high 5.1 (Chromecast with Google TV)
            "hvc1.1.6.L153.B0", // H.265 main 5.1 (Chromecast Ultra)
            "mp4a.40.2",        // AAC LC
            "mp4a.40.5",        // AAC HE
            "mp4a.69",          // MP3
            "mp4a.6B",          // MP3
        ];

        /**
         * Supported modes (webrtc, mse, mp4, mjpeg).
         * @type {string}
         */
        this.mode = "webrtc,mse,mp4,mjpeg";

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

        /**
         * Handler for receiving Binary from WebSocket
         * @type {Function}
         */
        this.ondata = null;

        /**
         * Handlers list for receiving JSON from WebSocket
         * @type {Object.<string,Function>}}
         */
        this.onmessage = null;
    }

    /** public properties **/

    /**
     * Set video source (WebSocket URL). Support relative path.
     * @param {string|URL} value
     */
    set src(value) {
        if (typeof value !== "string") value = value.toString();
        if (value.startsWith("http")) {
            value = "ws" + value.substring(4);
        } else if (value.startsWith("/")) {
            value = "ws" + location.origin.substring(4) + value;
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
        this.internalConnect();
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

        // important for second video for mode MP4
        this.style.display = "block";
        this.style.position = "relative";

        this.video.style.display = "block"; // fix bottom margin 4px
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

    internalConnect() {
        if (this.wsState !== WebSocket.CONNECTING) return;
        if (this.ws) throw "connect with non null WebSocket";

        const ts = Date.now();

        this.ws = new WebSocket(this.url);
        this.ws.binaryType = "arraybuffer";

        this.ws.addEventListener("open", () => {
            console.debug("VideoRTC.ws.open", this.wsState);

            // CONNECTING => OPEN
            this.wsState = WebSocket.OPEN;

            this.internalOpen();
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
                this.internalConnect();
            }, delay);
        });
    }

    internalOpen() {
        this.ws.addEventListener("message", ev => {
            if (typeof ev.data === "string") {
                const msg = JSON.parse(ev.data);
                for (const mode in this.onmessage) {
                    this.onmessage[mode](msg);
                }
            } else {
                this.ondata(ev.data);
            }
        });

        this.ondata = null;
        this.onmessage = {};

        let firstMode = "";

        if (this.mode.indexOf("mse") >= 0 && "MediaSource" in window) { // iPhone
            firstMode ||= "mse";
            this.internalMSE();
        } else if (this.mode.indexOf("mp4") >= 0) {
            firstMode ||= "mp4";
            this.internalMP4();
        }

        if (this.mode.indexOf("webrtc") >= 0 && "RTCPeerConnection" in window) { // macOS Desktop app
            firstMode ||= "webrtc";
            this.internalRTC();
        }

        if (this.mode.indexOf("mjpeg") >= 0) {
            if (firstMode) {
                this.onmessage["mjpeg"] = msg => {
                    if (msg.type !== "error" || msg.value.indexOf(firstMode) !== 0) return;
                    this.internalMJPEG();
                }
            } else {
                this.internalMJPEG();
            }
        }
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

        this.onmessage["mse"] = msg => {
            if (msg.type !== "mse") return;

            const sb = ms.addSourceBuffer(msg.value);
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

            const buf = new Uint8Array(2 * 1024 * 1024);
            let bufLen = 0;

            this.ondata = data => {
                if (sb.updating || bufLen > 0) {
                    const b = new Uint8Array(data);
                    buf.set(b, bufLen);
                    bufLen += b.byteLength;
                    // console.debug("VideoRTC.buffer", b.byteLength, bufLen);
                } else {
                    try {
                        sb.appendBuffer(data);
                    } catch (e) {
                        // console.debug(e);
                    }
                }
            }
        }
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

        this.onmessage["webrtc"] = msg => {
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
                case "error":
                    if (msg.value.indexOf("webrtc/offer") < 0) return;
                    pc.close();
            }
        };

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

        this.ondata = data => {
            this.video.poster = "data:image/jpeg;base64," + VideoRTC.btoa(data);
        };

        this.send({type: "mjpeg"});
        this.video.controls = false;
    }

    internalMP4() {
        console.debug("VideoRTC.internalMP4");

        /** @type {HTMLVideoElement} */
        let video2;

        this.ondata = data => {
            // first video with default position (set container size)
            // second video with position=absolute and top=0px
            if (video2) {
                this.removeChild(this.video);
                this.video.src = "";
                this.video = video2;
                video2.style.position = "";
                video2.style.top = "";
            }

            video2 = this.video.cloneNode();
            video2.style.position = "absolute";
            video2.style.top = "0px";
            this.appendChild(video2);

            video2.src = "data:video/mp4;base64," + VideoRTC.btoa(data);
            video2.play().catch(() => console.log);
        };

        this.ws.addEventListener("close", () => {
            if (!video2) return;

            this.removeChild(video2);
            video2.src = "";
        });

        this.send({type: "mp4", value: this.codecs("mp4")});
        this.video.controls = false;
    }

    static btoa(buffer) {
        const bytes = new Uint8Array(buffer);
        const len = bytes.byteLength;
        let binary = "";
        for (let i = 0; i < len; i++) {
            binary += String.fromCharCode(bytes[i]);
        }
        return window.btoa(binary);
    }
}

customElements.define("video-rtc", VideoRTC);
