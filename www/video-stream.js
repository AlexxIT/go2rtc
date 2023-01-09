import {VideoRTC} from "./video-rtc.js";

class VideoStream extends VideoRTC {
    set divMode(value) {
        this.querySelector(".mode").innerText = value;
        this.querySelector(".status").innerText = "";
    }

    set divError(value) {
        const state = this.querySelector(".mode").innerText;
        if (state !== "loading") return;
        this.querySelector(".mode").innerText = "error";
        this.querySelector(".status").innerText = value;
    }

    /**
     * Custom GUI
     */
    oninit() {
        console.debug("stream.oninit");
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

        const info = this.querySelector(".info")
        this.insertBefore(this.video, info);
    }

    onconnect() {
        console.debug("stream.onconnect");
        const result = super.onconnect();
        if (result) this.divMode = "loading";
        return result;
    }

    ondisconnect() {
        console.debug("stream.ondisconnect");
        super.ondisconnect();
    }

    onopen() {
        console.debug("stream.onopen");
        const result = super.onopen();

        this.onmessage["stream"] = msg => {
            console.debug("stream.onmessge", msg);
            switch (msg.type) {
                case "error":
                    this.divError = msg.value;
                    break;
                case "mse":
                case "mp4":
                case "mjpeg":
                    this.divMode = msg.type.toUpperCase();
                    break;
            }
        }

        return result;
    }

    onclose() {
        console.debug("stream.onclose");
        return super.onclose();
    }

    onpcvideo(ev) {
        console.debug("stream.onpcvideo");
        super.onpcvideo(ev);

        if (this.pcState !== WebSocket.CLOSED) {
            this.divMode = "RTC";
        }
    }
}

customElements.define("video-stream", VideoStream);
