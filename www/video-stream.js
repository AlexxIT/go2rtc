import {VideoRTC} from "./video-rtc.js";

class VideoStream extends VideoRTC {
    constructor() {
        super();

        /** @type {HTMLDivElement} */
        this.divMode = null;
        /** @type {HTMLDivElement} */
        this.divStatus = null;
    }

    /**
     * Custom GUI
     */
    oninit() {
        super.oninit();

        this.innerHTML = `
        <style>
        .info {
            position: absolute;
            top: 0;
            left: 0;
            right: 0;
            padding: 12px;
            color: white;
            display: flex;
            justify-content: space-between;
            pointer-events: none;
        }
        </style>
        <div class="info">
            <div class="status"></div>
            <div class="mode"></div>
        </div>
        `;

        this.divStatus = this.querySelector(".status");
        this.divMode = this.querySelector(".mode");

        const info = this.querySelector(".info")
        this.insertBefore(this.video, info);
    }

    onconnect() {
        const result = super.onconnect();
        if (result) {
            this.divMode.innerText = "loading";
        }
        return result;
    }

    onopen() {
        const result = super.onopen();

        this.onmessage["stream"] = msg => {
            switch (msg.type) {
                case "error":
                    this.divMode.innerText = "error";
                    this.divStatus.innerText = msg.value;
                    break;
                case "mse":
                case "mp4":
                case "mjpeg":
                    this.divMode.innerText = msg.type.toUpperCase();
                    this.divStatus.innerText = "";
                    break;
            }
        }

        return result;
    }

    onpcvideo(ev) {
        super.onpcvideo(ev);

        if (this.pcState !== WebSocket.CLOSED) {
            this.divMode.innerText = "RTC";
            this.divStatus.innerText = "";
        }
    }
}

customElements.define("video-stream", VideoStream);
