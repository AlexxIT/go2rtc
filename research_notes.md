# Wyze-Android-Bypass MVP Research Notes

## Core Requirements & Constraints

1.  **Objective**: Develop a native Android application (Wyze-Android-Bypass MVP) that authenticates and maintains a stream for a single Wyze doorbell camera.
2.  **Environment**: APatch-rooted ARM64 devices.
3.  **Key Functionalities**:
    *   Authentication.
    *   Stream maintenance (via a go2rtc bridge).
    *   Local network interception (preventing LAN interference by isolating the bridge using the Wyze UID and MAC address).
    *   Token scraping via APatch/su if the official Wyze app is installed (zero assumptions about the database structure).
    *   Crash logging with high verbosity for debug versions.

## Current Wyze Ecosystem (2026+) Intelligence

Based on the `go2rtc` implementation and recent repository context:

### Authentication & API
*   Wyze has an official Developer Portal allowing the generation of an `api_key` and `api_id` which must be paired with an email and password to fetch camera details.
*   **Endpoints:**
    *   Auth: `https://auth-prod.api.wyze.com`
    *   API: `https://api.wyzecam.com`
*   **App Identification:** `com.hualai.WyzeCam`

### Protocol & Streaming
*   Cameras use native P2P (TUTK / Gwell) protocol.
*   **DTLS Requirement:** Modern Wyze firmware *requires* DTLS encryption for local streaming.
*   **Stream URL Structure:** `wyze://[IP]?uid=[P2P_ID]&enr=[ENR]&mac=[MAC]&model=[MODEL]&subtype=[hd|sd]&dtls=true`
*   **Key Parameters Needed for Local Stream:**
    1.  `uid` (20-character P2P identifier)
    2.  `enr` (Encryption key for DTLS and Challenge-Response)
    3.  `mac` (Device MAC address)
    4.  `model` (e.g., `HL_CAM4`, `HL_DB2` for Doorbell v2)
*   **Authentication Challenge:** The connection involves a K10001/K10003 challenge-response mechanism using the `enr` key (either 16-byte or 32-byte derivations depending on the status code).
*   **Codecs:** Supports H.264/H.265 video; AAC, G.711, PCM, Opus audio. Two-way audio (intercom) is also supported by the protocol.

### Token & Database Scraping (Root/APatch)
*   Because the Wyze app's package name is `com.hualai.WyzeCam`, any local scraping via root (`su`) will need to target `/data/data/com.hualai.WyzeCam/`.
*   *Unknowns:* The exact structure of the SQLite database or SharedPreferences where the `enr`, `uid`, and `mac` are stored in 2026.
*   *Mitigation:* The MVP must include a generic SQLite/XML parser that dumps the contents of `/data/data/com.hualai.WyzeCam/` and searches for specific patterns (e.g., 20-character strings for UID, MAC address regex, strings resembling the `enr` key). If scraping fails or is blocked by local encryption (e.g., EncryptedSharedPreferences tied to the Android Keystore), the application must fallback to manual user entry via a Jetpack Compose form, requiring the user to obtain these details via the Wyze web API.

## go2rtc Bridging
*   The `go2rtc` bridge currently establishes the P2P connection, performs the K10001/K10003 DTLS handshake, and extracts the raw A/V packets.
*   To prevent LAN interference, the MVP needs to instantiate the go2rtc bridge with a configuration that strictly binds to the specific `uid` and `mac`.
*   Cross-compilation for `aarch64` will be necessary to embed `go2rtc` within the Android application.
