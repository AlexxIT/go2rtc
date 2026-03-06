# hksv - HomeKit Secure Video Library for Go

`hksv` is a standalone Go library that implements HomeKit Secure Video (HKSV) recording, motion detection, and HAP (HomeKit Accessory Protocol) camera server functionality. It can be used independently of go2rtc in any Go project that needs HKSV support.

## Author

Sergei "svk" Krashevich <svk@svk.su>

## Features

- **HKSV Recording** - Fragmented MP4 (fMP4) muxing with GOP-based buffering, sent over HDS (HomeKit DataStream)
- **Motion Detection** - P-frame size analysis using EMA (Exponential Moving Average) baseline with configurable threshold
- **HAP Server** - Full HomeKit pairing (SRP), encrypted communication, accessory management
- **Proxy Mode** - Transparent proxy for existing HomeKit cameras
- **Live Streaming** - Pluggable interface for RTP/SRTP live view (bring your own implementation)
- **Zero internal dependencies** - Only depends on `pkg/` packages, never on `internal/`

## Architecture

```
pkg/hksv/
    hksv.go          - Server, Config, interfaces (StreamProvider, PairingStore, etc.)
    consumer.go      - HKSVConsumer: fMP4 muxer + GOP buffer + HDS sender
    session.go       - hksvSession: HDS DataStream lifecycle management
    motion.go        - MotionDetector: P-frame based motion detection
    helpers.go       - Helper functions for ID/name generation
    consumer_test.go - Consumer tests and benchmarks
    motion_test.go   - Motion detector tests and benchmarks
```

### Dependency Graph

```
pkg/hksv/
  -> pkg/core         (Consumer, Connection, Media, Codec, Receiver, Sender)
  -> pkg/hap          (Server, Conn, Accessory, Character)
  -> pkg/hap/hds      (Conn, Session - encrypted DataStream)
  -> pkg/hap/camera   (TLV8 structs, services, accessory factories)
  -> pkg/hap/tlv8     (marshal/unmarshal)
  -> pkg/homekit      (ServerHandler, ProxyHandler, HandlerFunc)
  -> pkg/mp4          (Muxer - fMP4)
  -> pkg/h264         (IsKeyframe, RTPDepay, RepairAVCC)
  -> pkg/aac          (RTPDepay)
  -> pkg/mdns         (ServiceEntry for mDNS advertisement)
  -> github.com/pion/rtp
  -> github.com/rs/zerolog
  -> ZERO imports from internal/
```

## Quick Start

### Minimal HKSV Camera

```go
package main

import (
    "net/http"

    "github.com/AlexxIT/go2rtc/pkg/hap"
    "github.com/AlexxIT/go2rtc/pkg/hksv"
    "github.com/AlexxIT/go2rtc/pkg/mdns"
    "github.com/rs/zerolog"
    "github.com/rs/zerolog/log"
)

func main() {
    logger := log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

    srv, err := hksv.NewServer(hksv.Config{
        StreamName: "my-camera",
        Pin:        "27041991",
        HKSV:       true,
        MotionMode: "detect",
        Streams:    &myStreamProvider{},
        Store:      &myPairingStore{},
        Snapshots:  &mySnapshotProvider{},
        Logger:     logger,
        Port:       8080,
    })
    if err != nil {
        logger.Fatal().Err(err).Msg("failed to create server")
    }

    // Register HAP endpoints
    http.HandleFunc(hap.PathPairSetup, func(w http.ResponseWriter, r *http.Request) {
        srv.Handle(w, r)
    })
    http.HandleFunc(hap.PathPairVerify, func(w http.ResponseWriter, r *http.Request) {
        srv.Handle(w, r)
    })

    // Advertise via mDNS
    entry := srv.MDNSEntry()
    go mdns.Serve(mdns.ServiceHAP, []*mdns.ServiceEntry{entry})

    // Start HTTP server
    logger.Info().Msg("HomeKit camera running on :8080")
    http.ListenAndServe(":8080", nil)
}
```

### HKSV Camera with Live Streaming

```go
srv, err := hksv.NewServer(hksv.Config{
    StreamName: "my-camera",
    Pin:        "27041991",
    HKSV:       true,
    MotionMode: "detect",

    // Required interfaces
    Streams:    &myStreamProvider{},
    Store:      &myPairingStore{},
    Snapshots:  &mySnapshotProvider{},
    LiveStream: &myLiveStreamHandler{}, // enables live view in Home app
    Logger:     logger,
    Port:       8080,
})
```

### Basic Camera (no HKSV, live streaming only)

```go
srv, err := hksv.NewServer(hksv.Config{
    StreamName: "basic-cam",
    Pin:        "27041991",
    HKSV:       false, // no HKSV recording

    Streams:    &myStreamProvider{},
    LiveStream: &myLiveStreamHandler{},
    Logger:     logger,
    Port:       8080,
})
```

### Proxy Mode (transparent proxy for existing HomeKit camera)

```go
srv, err := hksv.NewServer(hksv.Config{
    StreamName: "proxied-cam",
    Pin:        "27041991",
    ProxyURL:   "homekit://192.168.1.100:51827?device_id=AA:BB:CC:DD:EE:FF&...",

    Logger: logger,
    Port:   8080,
})
```

### HomeKit Doorbell

```go
srv, err := hksv.NewServer(hksv.Config{
    StreamName: "my-doorbell",
    Pin:        "27041991",
    CategoryID: "doorbell", // creates doorbell accessory
    HKSV:       true,
    MotionMode: "detect",

    Streams:   &myStreamProvider{},
    Store:     &myPairingStore{},
    Snapshots: &mySnapshotProvider{},
    Logger:    logger,
    Port:      8080,
})

// Trigger doorbell press from external event
srv.TriggerDoorbell()
```

## Interfaces

The library uses dependency injection via four interfaces. You implement these to connect `hksv` to your own stream management, storage, and media pipeline.

### StreamProvider (required)

Connects HKSV consumers to your video/audio streams.

```go
type StreamProvider interface {
    // AddConsumer connects a consumer to the named stream.
    // The consumer implements core.Consumer (AddTrack, WriteTo, Stop).
    AddConsumer(streamName string, consumer core.Consumer) error

    // RemoveConsumer disconnects a consumer from the named stream.
    RemoveConsumer(streamName string, consumer core.Consumer)
}
```

**Example implementation:**

```go
type myStreamProvider struct {
    streams map[string]*Stream // your stream registry
}

func (p *myStreamProvider) AddConsumer(name string, cons core.Consumer) error {
    stream, ok := p.streams[name]
    if !ok {
        return fmt.Errorf("stream not found: %s", name)
    }
    return stream.AddConsumer(cons)
}

func (p *myStreamProvider) RemoveConsumer(name string, cons core.Consumer) {
    if stream, ok := p.streams[name]; ok {
        stream.RemoveConsumer(cons)
    }
}
```

### PairingStore (optional)

Persists HomeKit pairing data across restarts. If `nil`, pairings are lost on restart and the device must be re-paired.

```go
type PairingStore interface {
    SavePairings(streamName string, pairings []string) error
}
```

**Example implementation (JSON file):**

```go
type filePairingStore struct {
    path string
}

func (s *filePairingStore) SavePairings(name string, pairings []string) error {
    data := map[string][]string{name: pairings}
    b, err := json.Marshal(data)
    if err != nil {
        return err
    }
    return os.WriteFile(s.path, b, 0644)
}
```

### SnapshotProvider (optional)

Generates JPEG snapshots for HomeKit `/resource` requests (shown in the Home app timeline and notifications). If `nil`, snapshots are not available.

```go
type SnapshotProvider interface {
    GetSnapshot(streamName string, width, height int) ([]byte, error)
}
```

**Example implementation (ffmpeg):**

```go
type ffmpegSnapshotProvider struct {
    streams map[string]*Stream
}

func (p *ffmpegSnapshotProvider) GetSnapshot(name string, w, h int) ([]byte, error) {
    stream := p.streams[name]
    if stream == nil {
        return nil, errors.New("stream not found")
    }

    // Capture one keyframe from the stream
    frame, err := stream.CaptureKeyframe()
    if err != nil {
        return nil, err
    }

    // Convert to JPEG using ffmpeg
    return ffmpegToJPEG(frame, w, h)
}
```

### LiveStreamHandler (optional)

Handles live-streaming requests from the Home app (RTP/SRTP setup). If `nil`, only HKSV recording is available (no live view).

```go
type LiveStreamHandler interface {
    // SetupEndpoints handles a SetupEndpoints request (HAP characteristic 118).
    // Creates the RTP/SRTP consumer, returns the response value.
    SetupEndpoints(conn net.Conn, offer *camera.SetupEndpointsRequest) (any, error)

    // GetEndpointsResponse returns the current endpoints response (for GET requests).
    GetEndpointsResponse() any

    // StartStream starts RTP streaming with the given configuration.
    // The connTracker is used to register/unregister the live stream connection
    // on the HKSV server (for connection tracking and MarshalJSON).
    StartStream(streamName string, conf *camera.SelectedStreamConfiguration, connTracker ConnTracker) error

    // StopStream stops a stream matching the given session ID.
    StopStream(sessionID string, connTracker ConnTracker) error
}

type ConnTracker interface {
    AddConn(v any)
    DelConn(v any)
}
```

**Example implementation (SRTP-based):**

```go
type srtpLiveStreamHandler struct {
    mu       sync.Mutex
    consumer *homekit.Consumer
    srtp     *srtp.Server
    streams  map[string]*Stream
}

func (h *srtpLiveStreamHandler) SetupEndpoints(conn net.Conn, offer *camera.SetupEndpointsRequest) (any, error) {
    consumer := homekit.NewConsumer(conn, h.srtp)
    consumer.SetOffer(offer)

    h.mu.Lock()
    h.consumer = consumer
    h.mu.Unlock()

    answer := consumer.GetAnswer()
    v, err := tlv8.MarshalBase64(answer)
    return v, err
}

func (h *srtpLiveStreamHandler) GetEndpointsResponse() any {
    h.mu.Lock()
    consumer := h.consumer
    h.mu.Unlock()
    if consumer == nil {
        return nil
    }
    answer := consumer.GetAnswer()
    v, _ := tlv8.MarshalBase64(answer)
    return v
}

func (h *srtpLiveStreamHandler) StartStream(streamName string, conf *camera.SelectedStreamConfiguration, ct hksv.ConnTracker) error {
    h.mu.Lock()
    consumer := h.consumer
    h.mu.Unlock()
    if consumer == nil {
        return errors.New("no consumer")
    }
    if !consumer.SetConfig(conf) {
        return errors.New("wrong config")
    }

    ct.AddConn(consumer)
    stream := h.streams[streamName]
    if err := stream.AddConsumer(consumer); err != nil {
        return err
    }

    go func() {
        _, _ = consumer.WriteTo(nil) // blocks until stream ends
        stream.RemoveConsumer(consumer)
        ct.DelConn(consumer)
    }()

    return nil
}

func (h *srtpLiveStreamHandler) StopStream(sessionID string, ct hksv.ConnTracker) error {
    h.mu.Lock()
    consumer := h.consumer
    h.mu.Unlock()
    if consumer != nil && consumer.SessionID() == sessionID {
        _ = consumer.Stop()
    }
    return nil
}
```

## Config Reference

```go
type Config struct {
    // Required
    StreamName string           // stream identifier (used for lookups)
    Pin        string           // HomeKit pairing PIN, e.g. "27041991" (default)
    Port       uint16           // HAP HTTP port
    Logger     zerolog.Logger   // structured logger
    Streams    StreamProvider   // stream registry (required for HKSV/live/motion)

    // Optional - server identity
    Name          string   // mDNS display name (auto-generated from DeviceID if empty)
    DeviceID      string   // MAC-like ID, e.g. "AA:BB:CC:DD:EE:FF" (auto-generated if empty)
    DevicePrivate string   // ed25519 private key hex (auto-generated if empty)
    CategoryID    string   // "camera" (default), "doorbell", "bridge", or numeric
    Pairings      []string // pre-existing pairings from storage

    // Optional - mode
    ProxyURL string // if set, acts as transparent proxy (no local accessory)
    HKSV     bool   // enable HKSV recording support

    // Optional - motion detection
    MotionMode      string  // "api" (external trigger), "continuous" (always on), "detect" (P-frame analysis)
    MotionThreshold float64 // ratio threshold for "detect" mode (default 2.0, lower = more sensitive)

    // Optional - hardware
    Speaker *bool // include Speaker service for 2-way audio (default false)

    // Optional - metadata
    UserAgent string // for mDNS TXTModel field
    Version   string // for accessory firmware version

    // Optional - persistence and features
    Store      PairingStore     // nil = pairings not persisted
    Snapshots  SnapshotProvider // nil = no snapshot support
    LiveStream LiveStreamHandler // nil = no live streaming (HKSV recording only)
}
```

## Motion Detection

The library includes a built-in P-frame based motion detector that works without any external motion detection system.

### How It Works

1. During a **warmup phase** (30 P-frames), the detector establishes a baseline average frame size using fast EMA (alpha=0.1).
2. After warmup, each P-frame size is compared against the baseline multiplied by the threshold.
3. If `frame_size > baseline * threshold`, motion is detected.
4. Motion stays active for a **hold period** (30 seconds) after the last trigger frame.
5. After motion ends, there is a **cooldown period** (5 seconds) before new motion can be detected.
6. The baseline is updated continuously with slow EMA (alpha=0.02) during idle periods.
7. FPS is recalibrated every 150 frames for accurate hold/cooldown timing.

### Motion Modes

| Mode | Description |
|------|-------------|
| `"api"` | Motion is triggered externally via `srv.SetMotionDetected(true/false)` |
| `"detect"` | Automatic P-frame analysis (starts on first Home Hub connection) |
| `"continuous"` | Always reports motion every 30 seconds (for testing/always-record) |

### Using the MotionDetector Standalone

The `MotionDetector` can be used independently as a `core.Consumer`:

```go
onMotion := func(detected bool) {
    if detected {
        log.Println("Motion started!")
        // start recording, send notification, etc.
    } else {
        log.Println("Motion ended")
    }
}

detector := hksv.NewMotionDetector(2.0, onMotion, logger)

// Attach to a stream (detector implements core.Consumer)
err := stream.AddConsumer(detector)

// Blocks until Stop() is called
go func() {
    detector.WriteTo(nil)
}()

// Later, stop the detector
detector.Stop()
```

## Server API

### Motion Control

```go
// Trigger motion detected (for "api" mode or external sensors)
srv.SetMotionDetected(true)

// Clear motion
srv.SetMotionDetected(false)

// Trigger doorbell press event
srv.TriggerDoorbell()
```

### Connection Tracking

```go
// Register a connection (for monitoring/JSON output)
srv.AddConn(conn)

// Unregister a connection
srv.DelConn(conn)
```

### Pairing Management

```go
// Add a new pairing (called automatically during HAP pair-setup)
srv.AddPair(clientID, publicKey, hap.PermissionAdmin)

// Remove a pairing
srv.DelPair(clientID)

// Get client's public key (used by HAP pair-verify)
pubKey := srv.GetPair(clientID)
```

### JSON Serialization

The server implements `json.Marshaler` for status reporting:

```go
b, _ := json.Marshal(srv)
// {"name":"go2rtc-A1B2","device_id":"AA:BB:CC:DD:EE:FF","paired":1,"category_id":"17","connections":[...]}

// If not paired, includes setup_code and setup_id for QR code generation
// {"name":"go2rtc-A1B2","device_id":"AA:BB:CC:DD:EE:FF","setup_code":"195-50-224","setup_id":"A1B2"}
```

### mDNS Advertisement

```go
entry := srv.MDNSEntry()

// Start mDNS advertisement
go mdns.Serve(mdns.ServiceHAP, []*mdns.ServiceEntry{entry})
```

## Helper Functions

For deterministic ID generation from stream names:

```go
// Generate a display name from a seed
name := hksv.CalcName("", "my-camera")
// => "go2rtc-A1B2" (deterministic from seed)

name = hksv.CalcName("My Camera", "")
// => "My Camera" (uses provided name)

// Generate a MAC-like device ID
deviceID := hksv.CalcDeviceID("", "my-camera")
// => "AA:BB:CC:DD:EE:FF" (deterministic from seed)

// Generate an ed25519 private key
privateKey := hksv.CalcDevicePrivate("", "my-camera")
// => []byte{...} (deterministic 64-byte ed25519 key)

// Generate a setup ID for QR codes
setupID := hksv.CalcSetupID("my-camera")
// => "A1B2"

// Convert category string to HAP constant
catID := hksv.CalcCategoryID("doorbell")
// => "18" (hap.CategoryDoorbell)
```

## Multiple Cameras

You can run multiple HKSV cameras on a single port. Each camera gets its own mDNS entry and is resolved by hostname:

```go
cameras := []string{"front-door", "backyard", "garage"}
var entries []*mdns.ServiceEntry

for _, name := range cameras {
    srv, _ := hksv.NewServer(hksv.Config{
        StreamName: name,
        Pin:        "27041991",
        HKSV:       true,
        MotionMode: "detect",
        Streams:    provider,
        Logger:     logger,
        Port:       8080,
    })

    entry := srv.MDNSEntry()
    entries = append(entries, entry)

    // Map hostname -> server for HTTP routing
    host := entry.Host(mdns.ServiceHAP)
    handlers[host] = srv
}

// Single HTTP server handles all cameras
http.HandleFunc(hap.PathPairSetup, func(w http.ResponseWriter, r *http.Request) {
    if srv := handlers[r.Host]; srv != nil {
        srv.Handle(w, r)
    }
})
http.HandleFunc(hap.PathPairVerify, func(w http.ResponseWriter, r *http.Request) {
    if srv := handlers[r.Host]; srv != nil {
        srv.Handle(w, r)
    }
})

go mdns.Serve(mdns.ServiceHAP, entries)
http.ListenAndServe(":8080", nil)
```

## HKSV Recording Flow

Understanding the recording flow helps with debugging:

```
1. Home Hub discovers camera via mDNS
2. Home Hub connects -> PairSetup (first time) or PairVerify (subsequent)
3. On PairVerify success:
   - If motion="detect": MotionDetector starts consuming the video stream
   - If motion="continuous": prepareHKSVConsumer() + startContinuousMotion()
4. Motion detected -> SetMotionDetected(true) -> HAP event notification
5. Home Hub receives motion event -> sets up HDS DataStream:
   - SetCharacteristic(TypeSetupDataStreamTransport) -> TCP listener created
   - Home Hub connects to TCP port -> encrypted HDS connection established
   - hksvSession created
6. Home Hub opens dataSend stream:
   - handleOpen() -> takes prepared consumer (or creates new one)
   - consumer.Activate() -> sends fMP4 init segment over HDS
   - H264 keyframes trigger GOP flush -> mediaFragment sent over HDS
7. Home Hub closes dataSend -> handleClose() -> consumer stopped
8. Motion timeout -> SetMotionDetected(false)
```

## Example CLI Application

The `example/` directory contains a standalone CLI app that exports any RTSP camera as an HKSV camera in HomeKit.

### Build & Run

```bash
# Run directly
go run ./pkg/hksv/example -url rtsp://camera:554/stream

# Or build a binary
go build -o hksv-camera ./pkg/hksv/example
./hksv-camera -url rtsp://admin:pass@192.168.1.100:554/h264
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-url` | (required) | RTSP stream URL |
| `-pin` | `27041991` | HomeKit pairing PIN |
| `-port` | `0` (auto) | HAP HTTP port |
| `-motion` | `detect` | Motion mode: `detect`, `continuous`, `api` |
| `-threshold` | `2.0` | Motion sensitivity (lower = more sensitive) |
| `-pairings` | `pairings.json` | File to persist HomeKit pairings |

### How It Works

1. Connects to the RTSP source, discovers available tracks (H264/AAC)
2. Creates an HKSV server with HAP pairing and encrypted communication
3. Advertises the camera via mDNS — it appears in the Home app
4. On motion detection, Home Hub opens an HDS DataStream and records fMP4 fragments
5. Pairings are saved to a JSON file so the camera survives restarts

### Architecture

```
RTSP Camera ──► rtsp.Conn (Producer)
                    │
                    ▼
              streamProvider ◄── hksv.Server
              (AddConsumer)       │       │
                    │             ▼       ▼
                    ├── MotionDetector   HKSVConsumer
                    │   (P-frame EMA)   (fMP4 → HDS)
                    │         │               │
                    │         ▼               ▼
                    │    HAP event →     Home Hub
                    │    motion notify   records video
                    │
                    └── mDNS advertisement
```

## Testing

```bash
# Run all tests
go test ./pkg/hksv/...

# Run with verbose output
go test -v ./pkg/hksv/...

# Run benchmarks
go test -bench=. ./pkg/hksv/...

# Run specific test
go test -v -run TestMotionDetector_BasicTrigger ./pkg/hksv/...
```

## Requirements

- Go 1.22+
- Dependencies: `github.com/pion/rtp`, `github.com/rs/zerolog` (plus go2rtc `pkg/` packages)
