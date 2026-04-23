# Master Roadmap: Wyze-Android-Bypass MVP

This document outlines the sequence of operations required to develop the Wyze-Android-Bypass MVP, a native Android application capable of local doorbell ownership via automated pipelines on APatch-rooted ARM64 devices.

## Phase 1: Core Architecture & Environment Setup

1.  **Project Initialization**
    *   Set up a new Android project using Kotlin and Jetpack Compose.
    *   Configure Gradle for NDK support (required for potential native libraries) and declare the required architecture targets (`arm64-v8a`).
2.  **go2rtc Cross-Compilation**
    *   Set up a build pipeline to cross-compile the `go2rtc` binary for `aarch64` Android.
    *   Ensure the binary includes the `wyze` module (`pkg/wyze`, `pkg/tutk`).
    *   Embed the compiled binary in the Android app's `assets` or `res/raw` directory.
3.  **Permissions & Manifest**
    *   Request necessary permissions: `INTERNET`, `ACCESS_NETWORK_STATE`, `WAKE_LOCK`, and `FOREGROUND_SERVICE` (for persistent stream maintenance).

## Phase 2: Root-Native Token Extraction Pipeline (APatch)

1.  **Root Access Validation**
    *   Implement a root check using APatch/libsu.
    *   If root is unavailable, gracefully degrade to the Manual Configuration fallback.
2.  **Target Package Discovery**
    *   Verify the installation of `com.hualai.WyzeCam`.
3.  **Data Extraction & Heuristic Parsing**
    *   **Crucial Insight:** We *cannot* assume the database schema.
    *   Execute root shell commands to copy `/data/data/com.hualai.WyzeCam/databases/` and `/data/data/com.hualai.WyzeCam/shared_prefs/` to the MVP's local cache.
    *   Implement heuristic scanners to sift through the copied SQLite databases and XML files.
    *   **Targets:**
        *   `MAC Address`: Regex `^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`
        *   `UID`: 20-character alphanumeric P2P identifier.
        *   `ENR`: Encryption key (look for standard lengths or specific neighboring keys in JSON/DB rows).
4.  **Local Encryption Blockage Mitigation**
    *   *Risk:* Wyze may use Android Keystore-backed `EncryptedSharedPreferences` or SQLCipher.
    *   *Fallback:* If the heuristic scan yields encrypted blobs or empty results, abort extraction and trigger the Manual Configuration UI.

## Phase 3: go2rtc Bridge Integration & Pipeline

1.  **Configuration Generation**
    *   Dynamically generate a `go2rtc.yaml` configuration file based on the extracted (or manually entered) parameters.
    *   Format: `wyze://[IP]?uid=[UID]&enr=[ENR]&mac=[MAC]&model=[MODEL]&dtls=true`
    *   Ensure the configuration binds specifically to the target doorbell's parameters to prevent LAN interference.
2.  **Process Management**
    *   Extract the `go2rtc` binary from assets to the app's internal storage (`getFilesDir()`) and mark it as executable.
    *   Spawn the `go2rtc` process using `java.lang.ProcessBuilder`.
    *   Implement robust process monitoring to restart `go2rtc` if it crashes.
3.  **Stream Routing**
    *   Configure `go2rtc` to expose the stream locally (e.g., via WebRTC or RTSP on `localhost:8554`).
    *   Integrate an Android media player (e.g., ExoPlayer) to consume the local stream.

## Phase 4: Persistent Lifecycle Management

1.  **Foreground Service**
    *   Implement an Android Foreground Service to host the `go2rtc` process. This ensures the bridge survives Activity destruction.
2.  **Wakelocks & Network Reliability**
    *   Acquire `PARTIAL_WAKE_LOCK` to prevent the CPU from sleeping while the stream is active.
    *   Implement a `ConnectivityManager.NetworkCallback` to detect network drops and automatically restart the `go2rtc` bridge and reconnect to the camera when the network returns.

## Phase 5: Debugging & Crash Remediation (MVP Focus)

1.  **Verbose Logging**
    *   Capture standard output and standard error from the `go2rtc` process.
    *   Log all heuristic parsing results, API responses, and lifecycle events.
2.  **Crash Reporting**
    *   Implement an uncaught exception handler.
    *   On crash, write the full logcat buffer, `go2rtc` logs, and device state to a file in external storage for easy retrieval by developers.

## Potential Vectors of Failure & Armor

1.  **Encrypted Local Data:** As noted, if `/data/data/com.hualai.WyzeCam/` is encrypted via hardware keystores, local token scraping is **IMPOSSIBLE**.
    *   *Armor:* The application must seamlessly transition to a Jetpack Compose form requesting the user to provide the API Key, API ID, Email, and Password (to fetch details via the Wyze Cloud API) OR the raw `UID`/`ENR`/`MAC`.
2.  **Protocol Changes:** Wyze frequently updates firmwares. The current `TUTK`/`DTLS` challenge-response (K10001/K10003) might change.
    *   *Armor:* Ensure the `go2rtc` version is easily updatable independently of the Android APK. Log the raw challenge bytes if authentication fails.
3.  **JNI/NDK Boundary Issues:** If `go2rtc` is eventually converted to a shared library (`.so`) accessed via JNI instead of a standalone executable, memory management and threading between Go and Kotlin will be complex.
    *   *Armor:* For the MVP, stick to executing the standalone binary via `ProcessBuilder`. It provides process isolation and avoids JNI complexity.
