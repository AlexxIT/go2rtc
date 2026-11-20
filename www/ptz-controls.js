const defaultIcon = `<div style="color: rgb(255, 255, 255);margin-top: 3px;"><svg stroke="currentColor" fill="currentColor" stroke-width="0" viewBox="0 0 24 24" height="20px" width="20px" xmlns="http://www.w3.org/2000/svg"><path fill="none" d="M0 0h24v24H0z"></path><path d="M15.54 5.54 13.77 7.3 12 5.54 10.23 7.3 8.46 5.54 12 2zm2.92 10-1.76-1.77L18.46 12l-1.76-1.77 1.76-1.77L22 12zm-10 2.92 1.77-1.76L12 18.46l1.77-1.76 1.77 1.76L12 22zm-2.92-10 1.76 1.77L5.54 12l1.76 1.77-1.76 1.77L2 12z"></path><circle cx="12" cy="12" r="3"></circle></svg></div>`;

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
    //Version 1 UI dev
    const templateDevUI = `
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
        button[data-dir="zoom-in"] { grid-column: 3; grid-row: 1; }
        button[data-dir="zoom-out"] { grid-column: 1; grid-row: 3; }
    </style>
    <button class="toggle-button">${defaultIcon}</button>
    <div class="controls">
        <button data-dir="up">↑</button>
        <button data-dir="left">←</button>
        <button data-dir="right">→</button>
        <button data-dir="down">↓</button>
        <button data-dir="zoom-in">+</button>
        <button data-dir="zoom-out">−</button>
    </div>
    `;

    //Version 2 v0.dev UI :-o
    const templateV0devUI = `
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

        .camera-container {
            position: relative;
            width: 100%;
            height: 100%;
        }

        .camera-feed {
            width: 100%;
            height: calc(100% - 50px);
            background-color: #222;
            position: relative;
            overflow: hidden;
        }

        .controls {
            position: absolute;
            bottom: 30px;
            right: 20px;
            width: 100px;
            height: 100px;
            background-color: rgba(255, 255, 255, 0.7);
            border-radius: 50%;
            display: flex;
            justify-content: center;
            align-items: center;
            visibility: hidden;
        }
        .controls.visible {
            opacity: 1;
            visibility: visible;
        }

        .ptz-center {
            width: 30px;
            height: 30px;
            background-color: #f00;
            border-radius: 50%;
            cursor: pointer;
        }

        .ptz-arrow {
            position: absolute;
            width: 0;
            height: 0;
            border-style: solid;
            cursor: pointer;
        }

        .ptz-up {
            top: 10px;
            left: 50%;
            transform: translateX(-50%);
            border-width: 0 10px 10px 10px;
            border-color: transparent transparent #666 transparent;
        }

        .ptz-right {
            right: 10px;
            top: 50%;
            transform: translateY(-50%);
            border-width: 10px 0 10px 10px;
            border-color: transparent transparent transparent #666;
        }

        .ptz-down {
            bottom: 10px;
            left: 50%;
            transform: translateX(-50%);
            border-width: 10px 10px 0 10px;
            border-color: #666 transparent transparent transparent;
        }

        .ptz-left {
            left: 10px;
            top: 50%;
            transform: translateY(-50%);
            border-width: 10px 10px 10px 0;
            border-color: transparent #666 transparent transparent;
        }

        .zoom-controls {
            position: absolute;
            bottom: 25px;
            right: 130px;
            display: flex;
            flex-direction: column;
            align-items: center;
            gap: 15px;
            visibility: hidden;
        }
        .zoom-controls.visible {
            opacity: 1;
            visibility: visible;
        }

        .zoom-button {
            width: 50px;
            height: 50px;
            background-color: rgba(255, 255, 255, 0.7);
            border-radius: 50%;
            display: flex;
            justify-content: center;
            align-items: center;
            cursor: pointer;
        }

        .zoom-icon {
            position: relative;
            width: 20px;
            height: 20px;
        }

        .zoom-in .zoom-icon::before,
        .zoom-in .zoom-icon::after {
            content: '';
            position: absolute;
            background-color: #333;
        }

        .zoom-in .zoom-icon::before {
            width: 12px;
            height: 2px;
            top: 9px;
            left: 4px;
        }

        .zoom-in .zoom-icon::after {
            width: 2px;
            height: 12px;
            top: 4px;
            left: 9px;
        }

        .zoom-out .zoom-icon::before {
            content: '';
            position: absolute;
            background-color: #333;
            width: 12px;
            height: 2px;
            top: 9px;
            left: 4px;
        }

        .zoom-label {
            color: #666;
            font-size: 14px;
            margin-top: 5px;
        }

        .nav-bar {
            display: flex;
            height: 50px;
            background-color: #000;
            border-top: 1px solid #333;
        }

        .nav-item {
            flex: 1;
            display: flex;
            justify-content: center;
            align-items: center;
            color: #fff;
            text-decoration: none;
            font-size: 14px;
            cursor: pointer;
        }

        .nav-item.active {
            color: #fff;
            border-bottom: 2px solid #fff;
        }
    </style>

    <button class="toggle-button">${defaultIcon}</button>
    <div class="controls">
        <div class="ptz-arrow ptz-up" data-dir="up"></div>
        <div class="ptz-arrow ptz-right" data-dir="right"></div>
        <div class="ptz-arrow ptz-down" data-dir="down"></div>
        <div class="ptz-arrow ptz-left" data-dir="left"></div>
        <div class="ptz-center"></div>
    </div>
    <div class="zoom-controls">
        <div class="zoom-button zoom-in" data-dir="zoom-in">
            <div class="zoom-icon"></div>
        </div>
        <div class="zoom-button zoom-out" data-dir="zoom-out">
            <div class="zoom-icon"></div>
        </div>
    </div>
    `;

    this.shadowRoot.innerHTML = templateV0devUI;
  }

  setupControls() {
    // Toggle button setup
    const toggleButton = this.shadowRoot.querySelector(".toggle-button");
    const controlsPanel = this.shadowRoot.querySelector(".controls");
    const zoomControlsPanel = this.shadowRoot.querySelector(".zoom-controls");

    toggleButton.addEventListener("click", () => {
      this.controlsVisible = !this.controlsVisible;
      toggleButton.innerHTML = this.controlsVisible ? "⟩" : defaultIcon;

      if (this.controlsVisible) {
        controlsPanel.classList.add("visible");
        zoomControlsPanel.classList.add("visible");
        this.resetHideTimeout();
      } else {
        controlsPanel.classList.remove("visible");
        zoomControlsPanel.classList.remove("visible");
        this.clearHideTimeout();
      }
    });

    // Direction buttons setup
    const buttons = this.shadowRoot.querySelectorAll(".controls div");
    const zoomButtons = this.shadowRoot.querySelectorAll(".zoom-controls div");

    // Helper function to setup button event handlers
    const setupButtonHandlers = (button) => {
      let isPressed = false;

      const handleMove = () => {
        if (!isPressed) return;

        const dir = button.dataset.dir;

        if (!dir) return;

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
          case "zoom-in":
            zoomSpeed = 0.5;
            break;
          case "zoom-out":
            zoomSpeed = -0.5;
            break;
        }

        console.log("PTZ Move", pan, tilt, zoomSpeed, dir, button);

        // Only provide visual feedback for pan/tilt movements
        if (pan !== 0 || tilt !== 0) {
          const center = this.shadowRoot.querySelector(".ptz-center");
          center.style.backgroundColor = "#a00";
        } else if (zoomSpeed !== 0) {
          const btZoom = this.shadowRoot.querySelector(`.${dir}`);
          if (btZoom !== null) {
            btZoom.style.backgroundColor = "#ddd";
          }
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
        const center = this.shadowRoot.querySelector(".ptz-center");
        center.style.backgroundColor = "#f00";
        const buttonIn = this.shadowRoot.querySelector(`.zoom-in`);
        buttonIn.style.backgroundColor = "rgba(255, 255, 255, 0.7)";
        const buttonOut = this.shadowRoot.querySelector(`.zoom-out`);
        buttonOut.style.backgroundColor = "rgba(255, 255, 255, 0.7)";
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
        const center = this.shadowRoot.querySelector(".ptz-center");
        center.style.backgroundColor = "#f00";
        const buttonIn = this.shadowRoot.querySelector(`.zoom-in`);
        buttonIn.style.backgroundColor = "rgba(255, 255, 255, 0.7)";
        const buttonOut = this.shadowRoot.querySelector(`.zoom-out`);
        buttonOut.style.backgroundColor = "rgba(255, 255, 255, 0.7)";
        this.resetHideTimeout();
      });
    };

    // Apply event handlers to all buttons
    buttons.forEach(setupButtonHandlers);
    zoomButtons.forEach(setupButtonHandlers);

    // Auto-hide on mouse movement outside the controls
    controlsPanel.addEventListener("mousemove", () => {
      this.resetHideTimeout();
    });

    zoomControlsPanel.addEventListener("mousemove", () => {
      this.resetHideTimeout();
    });

    // Reset hide timeout when hovering over the controls
    controlsPanel.addEventListener("mouseenter", () => {
      this.resetHideTimeout();
    });

    zoomControlsPanel.addEventListener("mouseenter", () => {
      this.resetHideTimeout();
    });
  }

  resetHideTimeout() {
    this.clearHideTimeout();
    this.hideTimeout = setTimeout(() => {
      const controlsPanel = this.shadowRoot.querySelector(".controls");
      const zoomControlsPanel = this.shadowRoot.querySelector(".zoom-controls");
      const toggleButton = this.shadowRoot.querySelector(".toggle-button");
      controlsPanel.classList.remove("visible");
      zoomControlsPanel.classList.remove("visible");
      toggleButton.innerHTML = defaultIcon;
      this.controlsVisible = false;
    }, 8000);
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
