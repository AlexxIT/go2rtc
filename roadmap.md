# Wyze-Android-Bypass MVP Roadmap

## 1. Project Overview
This MVP transforms a fork of `go2rtc` into a native Android application designed for local doorbell ownership via automated pipelines on APatch-rooted ARM64 devices. It bridges Wyze's proprietary P2P protocol locally using root-native extraction to avoid redundant cloud authentications.

## 2. Sequence of Operations

### Phase 1: Environment & Permissions Initialization
- **Root Acquisition:** The app requests root privileges via `su` (APatch compatible).
- **Network Permissions:** Request standard Android Internet/Network State permissions to bind to the local network.
- **File System Access:** Gain read access to `/data/data/` for cross-application intelligence gathering.

### Phase 2: Token & Device Data Extraction (The Scraping Engine)
- **Target Identification:** Attempt to locate the official Wyze application package (e.g., `com.hualai` or similar 2026 variants).
- **SQLite/Preferences Parsing:**
  - Read SharedPreferences XML or SQLite DB files within the Wyze app's secure data directory.
  - Search for `access_token`, `refresh_token`, and device arrays.
  - **Required Extraction Targets for go2rtc:**
    - `mac` (Device MAC address)
    - `uid` (20-character P2P identifier)
    - `enr` (DTLS Encryption Key)
    - `model` (Camera Model)
- **Fallback Mechanism:** If the official app is not installed, or data is encrypted/inaccessible due to newer security measures, present a Jetpack Compose form for manual Wyze API credential entry (API ID, API Key, Email, Password).

### Phase 3: Dynamic go2rtc Configuration Pipeline
- **YAML Generation:** Based on the extracted data, construct a dynamic `go2rtc.yaml` file in the app's isolated storage directory (`/data/data/com.wyzebypass.mvp/files/`).
- **Stream Construction:** Format the target string: `wyze://[IP]?uid=[P2P_ID]&enr=[ENR]&mac=[MAC]&model=[MODEL]&dtls=true`.
- **IP Resolution:** Resolve the camera's local IP address using ARP table scanning based on the extracted `mac` address.

### Phase 4: Native go2rtc Execution
- **AArch64 Compilation:** `go2rtc` must be cross-compiled to an `aarch64` shared library (`.so`) or executable wrapper callable via Android JNI/NDK.
- **Process Spawning:** Launch the go2rtc process pointing to the dynamically generated YAML.
- **Port Binding:** go2rtc exposes the stream locally on the Android device (e.g., via WebRTC or RTSP on `localhost:8554`).

### Phase 5: Stream Consumption & Lifecycle Management
- **Video Player:** Bind an Android video player (e.g., ExoPlayer for RTSP or WebRTC WebView) to the localhost stream.
- **Lifecycle Persistence:**
  - Implement Foreground Services with persistent notifications to keep the bridge alive in the background.
  - Utilize `WakeLocks` to prevent the CPU from sleeping during stream maintenance.
  - Implement a `BroadcastReceiver` to restart the service on network changes or drops.

## 3. Potential Vectors of Failure & Armouring

1. **Database Encryption:**
   - *Risk:* In 2026, Wyze may have moved to SQLCipher or Keystore-backed encryption for their `/data/data/` storage.
   - *Mitigation:* If scraping fails, the app must gracefully degrade to the manual Wyze API Key login form. Do not crash on inaccessible databases. Implement robust Try/Catch blocks during the SQLite reading phase.

2. **JNI/NDK Boundary Crashes:**
   - *Risk:* Running a Go binary (`go2rtc`) inside an Android context via JNI can lead to signal faults (SIGSEGV) if memory boundaries are crossed or if Go tries to use unavailable Android syscalls.
   - *Mitigation:* The MVP is a debug build. Configure the Go runtime and Android NDK to dump verbose tombstone crash logs to the app's file directory. Use `panic` recovery in the Go wrapper.

3. **DTLS Certificate Pinning / Protocol Changes:**
   - *Risk:* The TUTK/Wyze P2P protocol is reverse-engineered. Firmware updates could alter the handshake or DTLS requirements.
   - *Mitigation:* Ensure the `go2rtc` core can be updated independently of the Android UI. Log the exact byte sequences of failed handshakes to assist in rapid patching.

4. **Network State Instability:**
   - *Risk:* Android aggressively kills background network connections.
   - *Mitigation:* The MVP MUST use a Foreground Service.
