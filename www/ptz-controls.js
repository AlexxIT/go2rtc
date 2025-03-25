const defaultIcon = `<div style="color: rgb(255, 255, 255);"><svg stroke="currentColor" fill="currentColor" stroke-width="0" viewBox="0 0 24 24" height="20px" width="20px" xmlns="http://www.w3.org/2000/svg"><path fill="none" d="M0 0h24v24H0z"></path><path d="M15.54 5.54 13.77 7.3 12 5.54 10.23 7.3 8.46 5.54 12 2zm2.92 10-1.76-1.77L18.46 12l-1.76-1.77 1.76-1.77L22 12zm-10 2.92 1.77-1.76L12 18.46l1.77-1.76 1.77 1.76L12 22zm-2.92-10 1.76 1.77L5.54 12l1.76 1.77-1.76 1.77L2 12z"></path><circle cx="12" cy="12" r="3"></circle></svg></div>`;

class PTZControls extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: "open" });
    this.controlsVisible = false;
    this.hideTimeout = null;
    this.render();
    this.setupControls();
  }

  render() {
    this.shadowRoot.innerHTML = `
    <style>
        :host {
            display: block;
            position: absolute;
            bottom: 40px;
            right: 5px;
            z-index: 1000;
        }
        .toggle-button {
            position: absolute;
            bottom: 10px;
            right: 10px;
            width: 30px;
            height: 30px;
            background: rgba(0, 0, 0, 0.6);
            color: white;
            border: none;
            border-radius: 50%;
            cursor: pointer;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 14px;
            z-index: 1001;
            transition: opacity 0.3s;
        }
        .toggle-button:hover {
            background: rgba(0, 0, 0, 0.8);
        }
        .controls {
            display: grid;
            grid-template-columns: 1fr 1fr 1fr;
            gap: 5px;
            width: fit-content;
            background: rgba(0, 0, 0, 0.6);
            padding: 10px;
            border-radius: 8px;
            opacity: 0;
            visibility: hidden;
            transition: opacity 0.3s, visibility 0.3s;
        }
        .controls.visible {
            opacity: 1;
            visibility: visible;
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
        button[data-dir="down"] { grid-column: 2; grid-row: 3; }
    </style>
    <button class="toggle-button">${defaultIcon}</button>
    <div class="controls">
        <button data-dir="up">↑</button>
        <button data-dir="left">←</button>
        <button data-dir="right">→</button>
        <button data-dir="down">↓</button>
    </div>
    `;
  }

  setupControls() {
    // Toggle button setup
    const toggleButton = this.shadowRoot.querySelector(".toggle-button");
    const controlsPanel = this.shadowRoot.querySelector(".controls");

    toggleButton.addEventListener("click", () => {
      this.controlsVisible = !this.controlsVisible;
      toggleButton.innerHTML = this.controlsVisible ? "⟩" : defaultIcon;

      if (this.controlsVisible) {
        controlsPanel.classList.add("visible");
        this.resetHideTimeout();
      } else {
        controlsPanel.classList.remove("visible");
        this.clearHideTimeout();
      }
    });

    // Direction buttons setup
    const buttons = this.shadowRoot.querySelectorAll(".controls button");
    buttons.forEach((button) => {
      let isPressed = false;

      const handleMove = () => {
        if (!isPressed) return;

        const dir = button.dataset.dir;

        let pan = 0,
          tilt = 0,
          zoomSpeed = 0;

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
        }

        this.sendCommand("move", pan, tilt, zoomSpeed);
        this.resetHideTimeout();
      };

      button.addEventListener("mousedown", () => {
        isPressed = true;
        handleMove();
      });

      button.addEventListener("mouseup", () => {
        isPressed = false;
        this.sendCommand("stop");
        this.resetHideTimeout();
      });

      button.addEventListener("mouseleave", () => {
        if (isPressed) {
          isPressed = false;
          this.sendCommand("stop");
        }
        this.resetHideTimeout();
      });

      // Touch support
      button.addEventListener("touchstart", (e) => {
        e.preventDefault();
        isPressed = true;
        handleMove();
        this.resetHideTimeout();
      });

      button.addEventListener("touchend", (e) => {
        e.preventDefault();
        isPressed = false;
        this.sendCommand("stop");
        this.resetHideTimeout();
      });
    });

    // Auto-hide on mouse movement outside the controls
    controlsPanel.addEventListener("mousemove", () => {
      this.resetHideTimeout();
    });

    // Reset hide timeout when hovering over the controls
    controlsPanel.addEventListener("mouseenter", () => {
      this.resetHideTimeout();
    });
  }

  resetHideTimeout() {
    this.clearHideTimeout();
    this.hideTimeout = setTimeout(() => {
      const controlsPanel = this.shadowRoot.querySelector(".controls");
      const toggleButton = this.shadowRoot.querySelector(".toggle-button");
      controlsPanel.classList.remove("visible");
      toggleButton.textContent = "⟨";
      this.controlsVisible = false;
    }, 5000);
  }

  clearHideTimeout() {
    if (this.hideTimeout) {
      clearTimeout(this.hideTimeout);
      this.hideTimeout = null;
    }
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
