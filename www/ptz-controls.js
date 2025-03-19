class PTZControls extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: "open" });
    this.render();
    this.setupControls();
  }

  render() {
    //Layout 1
    this.shadowRoot.innerHTML = `
            <style>
                :host {
                    display: block;
                    position: absolute;
                    bottom: 40px;
                    right: 20px;
                    background: rgba(0, 0, 0, 0.6);
                    padding: 10px;
                    border-radius: 8px;
                    z-index: 1000;
                }
                .controls {
                    display: grid;
                    grid-template-columns: 1fr 1fr 1fr;
                    gap: 5px;
                    width: fit-content;
                }
                button {
                    width: 40px;
                    height: 40px;
                    border: none;
                    background: rgba(255, 255, 255, 0.2);
                    color: white;
                    border-radius: 4px;
                    cursor: pointer;
                    display: flex;
                    align-items: center;
                    justify-content: center;
                }
                button:hover {
                    background: rgba(255, 255, 255, 0.3);
                }
                button[data-dir="up"] { grid-column: 2; }
                button[data-dir="left"] { grid-column: 1; grid-row: 2; }
                button[data-dir="stop"] { grid-column: 2; grid-row: 2; }
                button[data-dir="right"] { grid-column: 3; grid-row: 2; }
                button[data-dir="down"] { grid-column: 2; grid-row: 3; }
            </style>
            <div class="controls">
                <button data-dir="up">↑</button>
                <button data-dir="left">←</button>
                <button data-dir="stop">□</button>
                <button data-dir="right">→</button>
                <button data-dir="down">↓</button>
            </div>
        `;
    //Layout 2
    this.shadowRoot.innerHTML = `
    <style>
        :host {
            display: block;
            position: absolute;
            bottom: 40px;
            right: 20px;
            background: rgba(0, 0, 0, 0.6);
            padding: 10px;
            border-radius: 8px;
            z-index: 1000;
        }
        .controls {
            display: grid;
            grid-template-columns: 1fr 1fr 1fr;
            gap: 5px;
            width: fit-content;
        }
        button {
            width: 40px;
            height: 40px;
            border: none;
            background: rgba(255, 255, 255, 0.2);
            color: white;
            border-radius: 4px;
            cursor: pointer;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 18px;
        }
        button:hover {
            background: rgba(255, 255, 255, 0.3);
        }
        button[data-dir="up"] { grid-column: 2; grid-row: 1; }
        button[data-dir="left"] { grid-column: 1; grid-row: 2; }
        button[data-dir="right"] { grid-column: 3; grid-row: 2; }
        button[data-dir="down"] { grid-column: 2; grid-row: 2; }
    </style>
    <div class="controls">
        <button data-dir="up">↑</button>
        <button data-dir="left">←</button>
        <button data-dir="down">↓</button>
        <button data-dir="right">→</button>
    </div>
    `;
  }

  setupControls() {
    const buttons = this.shadowRoot.querySelectorAll("button");
    buttons.forEach((button) => {
      let isPressed = false;

      const handleMove = () => {
        if (!isPressed) return;

        const dir = button.dataset.dir;
        const zoom = button.dataset.zoom;

        if (dir === "stop" || !isPressed) {
          this.sendCommand("stop");
          return;
        }

        let pan = 0,
          tilt = 0,
          zoomSpeed = 0;

        if (zoom) {
          zoomSpeed = zoom === "in" ? 0.5 : -0.5;
        } else {
          switch (dir) {
            case "up":
              tilt = 0.5;
              break;
            case "down":
              tilt = -0.5;
              break;
            case "left":
              pan = -0.5;
              break;
            case "right":
              pan = 0.5;
              break;
            case "up-left":
              pan = -0.5;
              tilt = 0.5;
              break;
            case "up-right":
              pan = 0.5;
              tilt = 0.5;
              break;
            case "down-left":
              pan = -0.5;
              tilt = -0.5;
              break;
            case "down-right":
              pan = 0.5;
              tilt = -0.5;
              break;
          }
        }

        this.sendCommand("move", pan, tilt, zoomSpeed);
      };

      button.addEventListener("mousedown", () => {
        isPressed = true;
        handleMove();
      });

      button.addEventListener("mouseup", () => {
        isPressed = false;
        this.sendCommand("stop");
      });

      button.addEventListener("mouseleave", () => {
        if (isPressed) {
          isPressed = false;
          this.sendCommand("stop");
        }
      });

      // Touch support
      button.addEventListener("touchstart", (e) => {
        e.preventDefault();
        isPressed = true;
        handleMove();
      });

      button.addEventListener("touchend", (e) => {
        e.preventDefault();
        isPressed = false;
        this.sendCommand("stop");
      });
    });
  }

  async sendCommand(action, pan = 0, tilt = 0, zoom = 0) {
    try {
      const response = await fetch("/api/ptz", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          src: this.getAttribute("source"),
          action,
          pan,
          tilt,
          zoom,
        }),
      });

      if (!response.ok) {
        console.error("PTZ command failed:", await response.text());
      }
    } catch (error) {
      console.error("Failed to send PTZ command:", error);
    }
  }
}

customElements.define("ptz-controls", PTZControls);
