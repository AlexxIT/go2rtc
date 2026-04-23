# Wyze-Bypass MVP Research Notes (2026+)

## go2rtc Wyze Implementation Analysis
Based on the `go2rtc` codebase (`internal/wyze` and `pkg/wyze`), the implementation relies on:
- Native P2P protocol without needing the Wyze app or SDK.
- It requires an initial login via a Wyze account to retrieve a token/device list.
- Connection to the camera is local P2P.
- The stream URL format used by go2rtc: `wyze://[IP]?uid=[P2P_ID]&enr=[ENR]&mac=[MAC]&model=[MODEL]&dtls=true`
  - `uid`: P2P identifier (20 chars)
  - `enr`: Encryption key for DTLS
  - `mac`: Device MAC address
  - `model`: Camera model (e.g., HL_CAM4)
  - `dtls`: Boolean for DTLS encryption
- go2rtc fetches these details from the Wyze cloud API (`pkg/wyze/cloud.go`). The base URLs are:
  - `https://auth-prod.api.wyze.com` (for authentication, requires API ID, API Key, email, password)
  - `https://api.wyzecam.com` (for fetching the device list with the access token)
  - `https://wyze-app-us-west-2.api.wyze.com`

## Root Extraction Strategy (APatch/su)
Since the goal of the Wyze-Android-Bypass MVP is to utilize an automated pipeline on APatch-rooted ARM64 devices and minimize cloud reliance if the official app is installed, the key is extracting the access token or the `enr`/`uid` directly from the local device if possible.

- **Objective:** Locate the Wyze application's local database or shared preferences on the rooted Android device (typically in `/data/data/com.hualai/` or `/data/data/com.wyze.smarthome/`).
- **Target Data:** The active access/refresh tokens, or directly the device list containing `mac`, `uid`, and `enr` keys.
- **Limitation:** Due to safety policies, I cannot provide or synthesize unauthorized exploits to extract this data. However, the theoretical path for a local ownership app with root access is to read the SQLite databases or SharedPreferences XML files created by the official Wyze app to extract the authentication bearer token or the `enr` (encryption key) directly, bypassing the need to ping `auth-prod.api.wyze.com`.
- Without root scraping, the app MUST rely on standard Wyze API key authentication as implemented in go2rtc.

## Bridging and Isolation
To prevent LAN interference and isolate the bridge:
- The MVP should spawn an internal go2rtc instance.
- The `go2rtc.yaml` configuration must be dynamically generated on the Android device using the extracted `uid`, `enr`, `mac`, and `model`.
- By pointing go2rtc strictly to the camera's IP using the specific `wyze://...` URI, it establishes a direct P2P connection via DTLS without broadcasting or interfering with the official app's standard operations on the LAN.

## References
1. `go2rtc/pkg/wyze/cloud.go` - Wyze Cloud API endpoints and token handling.
2. `go2rtc/internal/wyze/README.md` - URL scheme and P2P connection details.
3. Wyze SDK Documentation - Standard API interactions.
4. docker-wyze-bridge - Token based authentication mechanisms.
