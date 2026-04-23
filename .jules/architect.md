# ARCHITECT'S JOURNAL

## Current Status
Research and roadmap generation for Wyze-Android-Bypass MVP.

## Missing Intelligence
1.  **2026 Database Schema:** We lack the exact table structures for `com.hualai.WyzeCam` SQLite databases. We do not know if the `enr`, `uid`, and `mac` are stored in plaintext, obfuscated, or strictly encrypted using Android Keystore.
2.  **Firmware Peculiarities:** The `go2rtc` README notes that "Gwell based protocols are not yet supported" and lists several cameras (Pan v4, OG, OG Telephoto, OG 2025) under this. We need to verify if any Wyze Doorbells use Gwell instead of TUTK. Currently, Video Doorbell v2 uses TUTK.

## Conflicting Documentation
1.  **Authentication Source:** The `go2rtc` README states: "Requires Wyze account. You need to login once via the WebUI to load your cameras... Internet access is only needed when loading cameras from your account. After that, all streaming is local P2P." However, our goal is *local token scraping* via APatch. If the app relies on scraping, the web API login might be entirely bypassed, contradicting the standard `go2rtc` flow. The roadmap accommodates this by generating the `wyze://` URL manually if scraping succeeds.

## Potential JNI/NDK Boundary Issues
*   While the roadmap currently dictates running `go2rtc` as a standalone binary via `ProcessBuilder`, future iterations might require tighter integration (e.g., getting the raw H.264 frames directly into an Android `MediaCodec` surface without the RTSP/WebRTC overhead).
*   If we move to JNI, we must handle Go's garbage collector interacting with Dalvik/ART. Passing large byte arrays (video frames) across the JNI boundary can cause significant performance overhead and memory leaks if not properly managed with direct `ByteBuffer`s.
*   **Recommendation for MVP:** Avoid JNI entirely. Use standard network loopback (`localhost`) to consume the stream from the embedded `go2rtc` process.
