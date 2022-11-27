/**
 * Common function for processing MSE and MSE2 data.
 * @param ms MediaSource
 */
function MediaSourceHandler(ms) {
    let sb, qb = [];

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
                    if (!sb.updating && qb.length > 0) {
                        try {
                            sb.appendBuffer(qb.shift());
                        } catch (e) {
                            // console.warn(e);
                        }
                    }
                });
            }
        } else if (sb.updating || qb.length > 0) {
            qb.push(ev.data);
            // console.debug("buffer:", qb.length);
        } else {
            try {
                sb.appendBuffer(ev.data);
            } catch (e) {
                // console.warn(e);
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
 */
class VideoRTC extends HTMLElement {
    DISCONNECT_TIMEOUT = 5000;
    RECONNECT_TIMEOUT = 30000;

    CODECS = [
        "avc1.640029",      // H.264 high 4.1 (Chromecast 1st and 2nd Gen)
        "avc1.64002A",      // H.264 high 4.2 (Chromecast 3rd Gen)
        "hvc1.1.6.L153.B0", // H.265 main 5.1 (Chromecast Ultra)
        "mp4a.40.2",        // AAC LC
        "mp4a.40.5",        // AAC HE
        "mp4a.69",          // MP3
        "mp4a.6B",          // MP3
    ];

    /**
     * Enable MediaSource in Workers mode.
     * @type {boolean}
     */
    MSE2 = true;

    /**
     * Run stream when not displayed on the screen. Default `false`.
     * @type {boolean}
     */
    background = false;

    /**
     * Run stream only when player in the viewport. Stop when user scroll out player.
     * Value is percentage of visibility from `0` (not visible) to `1` (full visible).
     * Default `0` - disable;
     * @type {number}
     */
    intersectionThreshold = 0;

    /**
     * Run stream only when browser page on the screen. Stop when user change browser
     * tab or minimise browser windows.
     * @type {boolean}
     */
    visibilityCheck = true;

    /**
     * @type {HTMLVideoElement}
     */
    video = null;

    /**
     * @type {RTCPeerConnection}
     */
    pc = null;

    /**
     * @type {WebSocket}
     */
    ws = null;

    /**
     * Internal WebSocket connection state. Values: CLOSED, CONNECTING, OPEN
     * @type {number}
     */
    wsState = WebSocket.CLOSED;

    /**
     * Internal WebSocket URL.
     * @type {string}
     */
    wsURL = "";

    /**
     * Internal disconnect TimeoutID.
     * @type {number}
     */
    disconnectTimeout = 0;

    /**
     * Internal reconnect TimeoutID.
     * @type {number}
     */
    reconnectTimeout = 0;

    constructor() {
        super();

        console.debug("this.constructor");

        this.video = document.createElement("video");
        this.video.controls = true;
        this.video.playsInline = true;
    }

    /** public properties **/

    /**
     * Set video source (WebSocket URL). Support relative path.
     * @param value
     */
    set src(value) {
        if (value.startsWith("/")) {
            value = "ws" + location.origin.substr(4) + value;
        } else if (value.startsWith("http")) {
            value = "ws" + value.substr(4);
        }

        this.wsURL = value;

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
                this.video.play();
            }
        });
    }

    get codecs() {
        return this.CODECS.filter(value => {
            return MediaSource.isTypeSupported(`video/mp4; codecs="${value}"`);
        }).join();
    }

    /**
     * `CustomElement`. Invoked each time the custom element is appended into a
     * document-connected element.
     */
    connectedCallback() {
        console.debug("this.connectedCallback", this.wsState);
        if (this.disconnectTimeout) {
            clearTimeout(this.disconnectTimeout);
            this.disconnectTimeout = 0;
        }

        // because video autopause on disconnected from DOM
        const seek = this.video.seekable;
        if (seek.length > 0) {
            this.video.currentTime = seek.end(seek.length - 1);
            this.play();
        }

        if (!this.wsURL || this.wsState !== WebSocket.CLOSED) return;

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
        console.debug("this.disconnectedCallback", this.wsState);
        if (this.background || this.disconnectTimeout ||
            this.wsState === WebSocket.CLOSED) return;

        this.disconnectTimeout = setTimeout(() => {
            if (this.reconnectTimeout) {
                clearTimeout(this.reconnectTimeout);
                this.reconnectTimeout = 0;
            }

            this.disconnectTimeout = 0;
            // CONNECTING, OPEN => CLOSED
            this.wsState = WebSocket.CLOSED;

            if (this.ws) {
                this.ws.close();
                this.ws = null;
            }
        }, this.DISCONNECT_TIMEOUT);
    }

    internalInit() {
        if (this.childElementCount) return;

        this.appendChild(this.video);

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

        this.ws = new WebSocket(this.wsURL);
        this.ws.binaryType = "arraybuffer";

        this.ws.addEventListener("open", () => {
            console.debug("ws.open", this.wsState);
            if (this.wsState !== WebSocket.CONNECTING) return;

            // CONNECTING => OPEN
            this.wsState = WebSocket.OPEN;
        });
        this.ws.addEventListener("close", () => {
            console.debug("ws.close", this.wsState);
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

        if ("MediaSource" in window) {
            if (MediaSource.canConstructInDedicatedWorker && this.MSE2) {
                this.internalMSE2();
            } else {
                this.internalMSE();
            }
        }

        // TODO: this.internalRTC();
    }

    internalMSE() {
        console.debug("this.internalMSE");
        this.ws.addEventListener("open", () => {
            const ms = new MediaSource();
            ms.addEventListener("sourceopen", () => {
                URL.revokeObjectURL(this.video.src);
                this.ws.send(JSON.stringify({type: "mse", value: this.codecs}));
            }, {once: true});

            this.video.src = URL.createObjectURL(ms);
            this.play();

            this.ws.addEventListener("message", MediaSourceHandler(ms));
        });
    }

    internalMSE2() {
        console.debug("this.internalMSE2");
        const worker = new Worker("video-rtc.js");
        worker.addEventListener("message", ev => {
            if (ev.data.type === "handle") {
                this.video.srcObject = ev.data.value;
                this.play();
            } else if (ev.data.type === "sourceopen") {
                this.ws.send(JSON.stringify({type: "mse", value: this.codecs}));
            }
        });

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
        if (!("RTCPeerConnection" in window)) return; // macOS Desktop app
    }
}

customElements.define("video-rtc", VideoRTC);
