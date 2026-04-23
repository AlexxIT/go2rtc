# Architect's Journal

## Intelligence Gathered
- The `go2rtc` codebase has a robust Wyze implementation (added in v1.9.14) that utilizes native P2P protocols with DTLS encryption.
- Connection requires a specific URL scheme: `wyze://[IP]?uid=[P2P_ID]&enr=[ENR]&mac=[MAC]&model=[MODEL]&dtls=true`.
- The primary cloud endpoints are `https://auth-prod.api.wyze.com` and `https://api.wyzecam.com`.
- Root extraction (APatch) will target `/data/data/` directories for the Wyze app (e.g., `com.wyze.smarthome`) to scrape SQLite databases or SharedPreferences for the `uid`, `mac`, and crucially, the `enr` (encryption key).

## Missing Intelligence / Conflicts
- **Database Encryption:** It is unknown if the 2026 version of the Wyze Android app utilizes SQLCipher or Android Keystore to encrypt the tokens within `/data/data/`. If it does, raw file extraction via `su` will fail without memory dumping or Frida-like hooking, necessitating the fallback to manual API key entry.
- **Package Name:** The exact package name of the 2026 Wyze app must be confirmed during the development phase (historically `com.hualai` or `com.wyze.smarthome`).

## Boundary Issues (JNI/NDK)
- The core of `go2rtc` is written in Go. Running this within an Android application will require compiling the Go code to a shared library (`.so`) using `gomobile` or `go build -buildmode=c-shared`.
- The Android UI will need to interface with this C-shared library via JNI.
- Handling network socket creation within the Go runtime inside the Android application sandbox may trigger SELinux denials or require specific network capabilities not granted by default, even with root.
- Background execution of the Go runtime will be heavily scrutinized by the Android OS; WakeLocks and Foreground Services are mandatory to prevent the process from being suspended.
