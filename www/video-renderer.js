/**
 * VideoRenderer — Cascading VideoFrame renderer: WebGPU → WebGL2 → Canvas 2D.
 *
 * Each tier uses its own canvas to avoid context-type locking.
 * Automatically initializes on first frame and downgrades on errors.
 *
 * Usage:
 *   const renderer = new VideoRenderer(container, {cascade: 'webgpu,webgl,2d'});
 *   // in VideoDecoder output callback:
 *   renderer.draw(frame);
 *   frame.close();
 *   // cleanup:
 *   renderer.destroy();
 */

const TIER_WEBGPU = 0;
const TIER_WEBGL2 = 1;
const TIER_CANVAS2D = 2;
const TIER_NAMES = ['WebGPU', 'WebGL2', 'Canvas2D'];
const TIER_MAP = {webgpu: TIER_WEBGPU, webgl: TIER_WEBGL2, '2d': TIER_CANVAS2D};

const WGSL_VERTEX = `
struct Out { @builtin(position) pos: vec4f, @location(0) uv: vec2f }
@vertex fn main(@builtin(vertex_index) i: u32) -> Out {
    var p = array<vec2f,6>(
        vec2f( 1, 1), vec2f( 1,-1), vec2f(-1,-1),
        vec2f( 1, 1), vec2f(-1,-1), vec2f(-1, 1));
    var u = array<vec2f,6>(
        vec2f(1,0), vec2f(1,1), vec2f(0,1),
        vec2f(1,0), vec2f(0,1), vec2f(0,0));
    return Out(vec4f(p[i],0,1), u[i]);
}`;

const WGSL_FRAGMENT = `
@group(0) @binding(0) var s: sampler;
@group(0) @binding(1) var t: texture_external;
@fragment fn main(@location(0) uv: vec2f) -> @location(0) vec4f {
    return textureSampleBaseClampToEdge(t, s, uv);
}`;

class WebGPUTier {
    constructor(canvas) {
        this.canvas = canvas;
        this.device = null;
        this.ctx = null;
        this.pipeline = null;
        this.sampler = null;
        this.format = null;
    }

    async init() {
        if (!navigator.gpu) { VideoRenderer.log('WebGPU: API not available'); return false; }
        try {
            VideoRenderer.log('WebGPU: requesting adapter...');
            const adapter = await navigator.gpu.requestAdapter();
            if (!adapter) { VideoRenderer.log('WebGPU: no adapter available'); return false; }
            const info = adapter.info || {};
            VideoRenderer.log('WebGPU: adapter:', info.vendor || '?', info.architecture || '?', info.description || '');

            this.device = await adapter.requestDevice();
            this.format = navigator.gpu.getPreferredCanvasFormat();

            this.ctx = this.canvas.getContext('webgpu');
            if (!this.ctx) {
                VideoRenderer.log('WebGPU: getContext("webgpu") returned null');
                this.device.destroy(); this.device = null; return false;
            }
            this.ctx.configure({device: this.device, format: this.format, alphaMode: 'opaque'});

            this.pipeline = this.device.createRenderPipeline({
                layout: 'auto',
                vertex: {
                    module: this.device.createShaderModule({code: WGSL_VERTEX}),
                    entryPoint: 'main',
                },
                fragment: {
                    module: this.device.createShaderModule({code: WGSL_FRAGMENT}),
                    entryPoint: 'main',
                    targets: [{format: this.format}],
                },
                primitive: {topology: 'triangle-list'},
            });

            this.sampler = this.device.createSampler({magFilter: 'linear', minFilter: 'linear'});
            VideoRenderer.log('WebGPU: initialized, format:', this.format);
            return true;
        } catch (e) {
            VideoRenderer.log('WebGPU: init failed:', e.message || e);
            this.device = null; this.ctx = null;
            return false;
        }
    }

    draw(frame, w, h) {
        if (this.canvas.width !== w || this.canvas.height !== h) {
            this.canvas.width = w; this.canvas.height = h;
        }
        const bind = this.device.createBindGroup({
            layout: this.pipeline.getBindGroupLayout(0),
            entries: [
                {binding: 0, resource: this.sampler},
                {binding: 1, resource: this.device.importExternalTexture({source: frame})},
            ],
        });
        const enc = this.device.createCommandEncoder();
        const pass = enc.beginRenderPass({colorAttachments: [{
            view: this.ctx.getCurrentTexture().createView(),
            loadOp: 'clear', storeOp: 'store',
        }]});
        pass.setPipeline(this.pipeline);
        pass.setBindGroup(0, bind);
        pass.draw(6);
        pass.end();
        this.device.queue.submit([enc.finish()]);
    }

    destroy() {
        try { if (this.device) this.device.destroy(); } catch {}
        this.device = null; this.ctx = null; this.pipeline = null; this.sampler = null;
    }
}

const GLSL_VERTEX = `#version 300 es
out vec2 vUV;
void main() {
    float x = float(gl_VertexID & 1) * 2.0;
    float y = float(gl_VertexID & 2);
    vUV = vec2(x, 1.0 - y);
    gl_Position = vec4(vUV * 2.0 - 1.0, 0.0, 1.0);
    vUV.y = 1.0 - vUV.y;
}`;

const GLSL_FRAGMENT = `#version 300 es
precision mediump float;
in vec2 vUV;
uniform sampler2D uTex;
out vec4 c;
void main() { c = texture(uTex, vUV); }`;

class WebGL2Tier {
    constructor(canvas) {
        this.canvas = canvas;
        this.gl = null;
        this.program = null;
        this.texture = null;
        this.lastW = 0;
        this.lastH = 0;
    }

    init() {
        try {
            this.gl = this.canvas.getContext('webgl2', {
                alpha: false, desynchronized: true, antialias: false,
                powerPreference: 'high-performance',
            });
        } catch (e) { VideoRenderer.log('WebGL2: getContext threw:', e.message || e); this.gl = null; }
        if (!this.gl) { VideoRenderer.log('WebGL2: not available'); return false; }

        const gl = this.gl;

        const vs = gl.createShader(gl.VERTEX_SHADER);
        gl.shaderSource(vs, GLSL_VERTEX);
        gl.compileShader(vs);
        if (!gl.getShaderParameter(vs, gl.COMPILE_STATUS)) {
            VideoRenderer.log('WebGL2: vertex shader error:', gl.getShaderInfoLog(vs));
            gl.deleteShader(vs); this.gl = null; return false;
        }

        const fs = gl.createShader(gl.FRAGMENT_SHADER);
        gl.shaderSource(fs, GLSL_FRAGMENT);
        gl.compileShader(fs);
        if (!gl.getShaderParameter(fs, gl.COMPILE_STATUS)) {
            VideoRenderer.log('WebGL2: fragment shader error:', gl.getShaderInfoLog(fs));
            gl.deleteShader(vs); gl.deleteShader(fs); this.gl = null; return false;
        }

        this.program = gl.createProgram();
        gl.attachShader(this.program, vs);
        gl.attachShader(this.program, fs);
        gl.linkProgram(this.program);
        gl.deleteShader(vs);
        gl.deleteShader(fs);

        if (!gl.getProgramParameter(this.program, gl.LINK_STATUS)) {
            VideoRenderer.log('WebGL2: program link error:', gl.getProgramInfoLog(this.program));
            gl.deleteProgram(this.program);
            this.gl = null; this.program = null; return false;
        }

        gl.useProgram(this.program);
        this.texture = gl.createTexture();
        gl.activeTexture(gl.TEXTURE0);
        gl.bindTexture(gl.TEXTURE_2D, this.texture);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
        gl.uniform1i(gl.getUniformLocation(this.program, 'uTex'), 0);

        const dbg = gl.getExtension('WEBGL_debug_renderer_info');
        const gpuName = dbg ? gl.getParameter(dbg.UNMASKED_RENDERER_WEBGL) : gl.getParameter(gl.RENDERER);
        VideoRenderer.log('WebGL2: initialized, GPU:', gpuName);
        return true;
    }

    draw(frame, w, h) {
        const gl = this.gl;
        if (w !== this.lastW || h !== this.lastH) {
            this.canvas.width = w; this.canvas.height = h;
            this.lastW = w; this.lastH = h;
            gl.viewport(0, 0, w, h);
        }
        gl.useProgram(this.program);
        gl.bindTexture(gl.TEXTURE_2D, this.texture);
        gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, frame);
        gl.drawArrays(gl.TRIANGLES, 0, 6);
    }

    destroy() {
        try {
            if (this.gl) {
                if (this.texture) this.gl.deleteTexture(this.texture);
                if (this.program) this.gl.deleteProgram(this.program);
                const ext = this.gl.getExtension('WEBGL_lose_context');
                if (ext) ext.loseContext();
            }
        } catch {}
        this.gl = null; this.program = null; this.texture = null;
    }
}

class Canvas2DTier {
    constructor(canvas) {
        this.canvas = canvas;
        this.ctx = null;
    }

    init() { return true; }

    draw(frame, w, h) {
        if (this.canvas.width !== w) this.canvas.width = w;
        if (this.canvas.height !== h) this.canvas.height = h;
        if (!this.ctx) this.ctx = this.canvas.getContext('2d');
        if (this.ctx) this.ctx.drawImage(frame, 0, 0, w, h);
    }

    destroy() { this.ctx = null; }
}

export class VideoRenderer {
    static log(msg, ...args) {
        console.debug('[WebCodecs]', msg, ...args);
    }

    /**
     * @param {HTMLElement} container — element to insert canvases into
     * @param {object} [options]
     * @param {string} [options.cascade='webgpu,webgl,2d'] — comma-separated tier names
     * @param {string} [options.canvasStyle] — CSS for created canvases
     */
    constructor(container, options = {}) {
        const cascade = options.cascade || 'webgpu,webgl,2d';
        const canvasStyle = options.canvasStyle || 'display:block;max-width:100%;max-height:100%';

        this._container = container;
        this._allowedTiers = cascade.split(',').map(s => TIER_MAP[s.trim()]).filter(t => t !== undefined);
        this._tier = -1; // not initialized
        this._initPromise = null;
        this._destroyed = false;

        const makeCanvas = () => {
            const c = document.createElement('canvas');
            c.style.cssText = canvasStyle;
            return c;
        };

        // Each tier gets its own canvas
        this._tiers = {
            [TIER_WEBGPU]: new WebGPUTier(makeCanvas()),
            [TIER_WEBGL2]: new WebGL2Tier(makeCanvas()),
            [TIER_CANVAS2D]: new Canvas2DTier(makeCanvas()),
        };

        this._activeCanvas = null;

        // WebGL2 context loss/restore
        const gl2Canvas = this._tiers[TIER_WEBGL2].canvas;
        gl2Canvas.addEventListener('webglcontextlost', (e) => {
            e.preventDefault();
            if (this._tier === TIER_WEBGL2) {
                this._tiers[TIER_WEBGL2].destroy();
                this._tier = this._nextAllowed(TIER_CANVAS2D);
                this._swapCanvas(this._tier);
                VideoRenderer.log('WebGL2 context lost, fallback to', TIER_NAMES[this._tier]);
            }
        });
        gl2Canvas.addEventListener('webglcontextrestored', () => {
            if (this._tier > TIER_WEBGL2 && this._allowedTiers.includes(TIER_WEBGL2)) {
                if (this._tiers[TIER_WEBGL2].init()) {
                    this._tier = TIER_WEBGL2;
                    this._swapCanvas(TIER_WEBGL2);
                    VideoRenderer.log('WebGL2 context restored');
                }
            }
        });

        VideoRenderer.log('cascade:', this._allowedTiers.map(t => TIER_NAMES[t]).join(' → '));
    }

    /** @returns {string} current tier name or 'none' */
    get currentTier() {
        return this._tier >= 0 ? TIER_NAMES[this._tier] : 'none';
    }

    /** @returns {HTMLCanvasElement|null} currently active canvas */
    get canvas() {
        return this._activeCanvas;
    }

    /**
     * Draw a VideoFrame. Does NOT close the frame — caller is responsible.
     * @param {VideoFrame} frame
     */
    draw(frame) {
        if (this._destroyed) return;
        const w = frame.displayWidth, h = frame.displayHeight;
        if (w === 0 || h === 0) return;

        // Already initialized — fast path
        if (this._tier >= 0) {
            try {
                this._tiers[this._tier].draw(frame, w, h);
            } catch (e) {
                this._downgrade(e);
                try {
                    this._tiers[this._tier].draw(frame, w, h);
                } catch (e2) {
                    this._tier = TIER_CANVAS2D;
                    this._swapCanvas(TIER_CANVAS2D);
                    VideoRenderer.log('renderer error, fallback to Canvas2D:', e2.message || e2);
                    this._tiers[TIER_CANVAS2D].draw(frame, w, h);
                }
            }
            return;
        }

        // Async init in progress — use Canvas2D temporarily
        if (this._initPromise) {
            this._tiers[TIER_CANVAS2D].draw(frame, w, h);
            if (!this._activeCanvas) this._swapCanvas(TIER_CANVAS2D);
            return;
        }

        // First frame — initialize
        VideoRenderer.log('first frame, resolution:', w + 'x' + h, '— initializing...');
        const first = this._allowedTiers[0] ?? TIER_CANVAS2D;

        if (first === TIER_WEBGPU && navigator.gpu) {
            this._initPromise = this._tiers[TIER_WEBGPU].init().then(ok => {
                if (this._destroyed) return;
                if (ok) {
                    this._tier = TIER_WEBGPU;
                    this._swapCanvas(TIER_WEBGPU);
                } else {
                    this._initSync(TIER_WEBGL2);
                }
                VideoRenderer.log('renderer ready:', TIER_NAMES[this._tier]);
                this._initPromise = null;
            });
            // Render first frames with Canvas2D while WebGPU inits
            this._tiers[TIER_CANVAS2D].draw(frame, w, h);
            this._swapCanvas(TIER_CANVAS2D);
        } else {
            this._initSync(first === TIER_WEBGPU ? TIER_WEBGL2 : first);
            VideoRenderer.log('renderer ready:', TIER_NAMES[this._tier]);
            this._tiers[this._tier].draw(frame, w, h);
            this._swapCanvas(this._tier);
        }
    }

    destroy() {
        this._destroyed = true;
        for (const tier of Object.values(this._tiers)) {
            tier.destroy();
            if (tier.canvas.parentElement) tier.canvas.remove();
        }
        this._activeCanvas = null;
        this._tier = -1;
    }

    _nextAllowed(minTier) {
        for (const t of this._allowedTiers) { if (t >= minTier) return t; }
        return TIER_CANVAS2D;
    }

    _initSync(startTier) {
        const tryGL = this._allowedTiers.includes(TIER_WEBGL2);
        if (startTier <= TIER_WEBGL2 && tryGL && this._tiers[TIER_WEBGL2].init()) {
            this._tier = TIER_WEBGL2;
        } else {
            this._tier = this._nextAllowed(TIER_CANVAS2D);
        }
    }

    _downgrade(error) {
        const oldTier = this._tier;
        this._tier = this._nextAllowed(oldTier + 1);
        if (this._tier === TIER_WEBGL2 && !this._tiers[TIER_WEBGL2].gl && !this._tiers[TIER_WEBGL2].init()) {
            this._tier = this._nextAllowed(TIER_CANVAS2D);
        }
        this._swapCanvas(this._tier);
        VideoRenderer.log('renderer error, downgrade:', TIER_NAMES[oldTier], '→', TIER_NAMES[this._tier], error.message || error);
    }

    _swapCanvas(tier) {
        const target = this._tiers[tier]?.canvas;
        if (!target || target === this._activeCanvas) return;

        if (this._activeCanvas) {
            this._activeCanvas.style.display = 'none';
        }

        if (target.parentElement !== this._container) {
            // Insert before the first hidden canvas or at the end
            const ref = this._activeCanvas || null;
            this._container.insertBefore(target, ref);
        }
        target.style.display = '';
        this._activeCanvas = target;
    }
}
