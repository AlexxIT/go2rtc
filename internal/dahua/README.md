# Dahua Two-Way Audio (NetSDK Backchannel)

Push audio to a Dahua camera speaker as a go2rtc **consumer** (send-only).

Single backend: Dahua **NetSDK** binary private protocol, TCP 37777 (`CLIENT_StartTalkEx`).
This is **not** DHIP (port 5000 JSON-RPC) and **not** go2rtc's `pkg/dvrip` (Xiongmai / XMeye /
Sofia hardware) — only the surface (37777 framing, `speak.*` RPC) looks similar, so it's named
`dahua` to avoid clashing. CGI and HTTP `/RPC2_Login` talk transports are out of scope.

## Tested devices

- **Dahua E4702 OEM** (target; two-way audio verified).
- Dahua's line is heavily fragmented (IPC / NVR / Lite / OEM rebrands); `Talk.General`,
  DHAV sub-header and codec ids may all differ. **Not claimed to support "all Dahua".**
  If a model is silent, try `encodeformat` (table below) before assuming a bug.

## Configuration

```yaml
streams:
  cam_talk:
    - dahua://admin:PASSWORD@192.168.1.108:37777?backchannel=0
```

Click the microphone on the WebRTC page for `cam_talk` to talk. The backchannel is the NetSDK
binary protocol on **TCP 37777** (no service discovery; the `host:port` in the URL is dialed as-is).
If a device does not answer on 37777, this is likely a firmware difference — Dahua's lineup is
heavily fragmented and not every model binds the binary talk protocol to 37777. CGI talk is out of scope.

## Parameters

| Param | Values | Default | Description |
|------|------|------|------|
| `backchannel` | int | `0` | Talk channel number, 0-based |
| `codec` | `pcma` / `pcmu` / `pcml` / `aac` | none | Pin the offered codec; empty offers PCMA/PCMU/PCML (no device probe) |
| `debug` | `1` | off | Dump every raw frame to the `dahua` log |
| `timeout` | seconds | `5` | Dial timeout |
| `encodeformat` | `0` / `1` / `2` / `4` | `1` | NetSDK only: `Talk.General` EncodeFormat (DH_TALK coding type, see table below); if silent, try `0`, `2` or `4` |
| `native` | `0` / `1` | `1` | NetSDK only: `1` = forward G.711 untouched (E4702 accepts G.711, half the bytes); `0` = decode to PCM16 first |

## Codec notes

The default offer collapses to **PCMA (G.711 A-law)**. `negotiableCodecs` keeps PCMA alone when
it is present in the candidate list (upstream's "leave only one codec here for better
compatibility" rule); any PCMU / PCML the caller also listed are dropped by that collapse. PCMA
leads because a WebRTC browser can only *originate* Opus / G.711 / G722, so A-law needs zero
transcoding, and the E4702 speaks A-law (nativeG711 forwards it untouched). The `Channels` field
is cleared, or ffmpeg Play answers `can't find consumer` ("talk works, Play doesn't"). **AAC**
(`codec=aac`) works but upstream flags "unknown problems on Dahua two way" — avoid as first
choice. **PCML** (`codec=pcml`) only negotiates with non-browser callers: no browser can *encode*
PCM16 (L16) over WebRTC, so pcml is for ffmpeg/file sources.

> **Naming trap.** `dahua://...?backchannel=0` means *enable talk channel 0* (talk is ON). This is
> the opposite of go2rtc's RTSP scheme `#backchannel=0`, which *disables* the WebRTC backchannel
> switch. Same word, opposite polarity — don't mix them up.

## Login lockout guard

Firmware enforces a per-session login lock after a few bad logins (E4702; validated). Only the offending TCP/login session is blocked, not the whole account — a new session with the correct password still authenticates. This is a PER-SESSION lock, NOT a global account lock. Because go2rtc
auto-reconnects, a wrong password can trip the lockout in seconds — and the lockout reports
"wrong password", same as the real error. A **30 s backoff brake** prevents retries on a
rejected credential set; fixing the password clears it immediately.

## NetSDK protocol reference

Source of truth: Dahua official **Win64 C NetSDK** (`General_NetSDK_Eng_Win64`, `dhnetsdk.h`) and
the official `Talk` MFC demo (`Demo/MfcDemo/05.Talk/TalkDlg.cpp`). The demo hardcodes
`encodeType = DH_TALK_PCM`, `dwSampleRate = 8000`, `nAudioBit = 16` and exposes no other option in
its UI; the E4702 answers this format (confirmed via `dvrip_proxy` capture of the vendor client).

### `encodeformat` (Talk.General EncodeFormat)

Maps to the SDK `DH_TALK_CODING_TYPE` enum:

| Value | SDK constant | Meaning |
|------|------|------|
| `0` | `DH_TALK_DEFAULT` | raw PCM (no DHAV audio sub-header) |
| `1` | `DH_TALK_PCM` | PCM16 **with** the DHAV sub-header (demo default; this fork's default). Note: the on-wire codec is chosen by `native`/`codec`; with `native=1` the device still sends G.711 A-law, not PCM16. |
| `2` | `DH_TALK_G711a` | G.711 A-law |
| `4` | `DH_TALK_G711u` | G.711 µ-law |

`1` is the default because it matches the official demo and the E4702 answers it. If a device is
silent on `1`, try `0`, `2` or `4` (unverified on E4702).

The SDK `DH_TALK_CODING_TYPE` enum defines more values (`3` = AMR, plus G726, G729, AAC, MP3, …) but this
fork emits PCM16 (`0x0C`), G.711 A-law (`0x0E`), G.711 µ-law (`0x0A`) and AAC (`0x1A`, opt-in via
`codec=aac`) on the wire; of the SDK enum, only `0`/`1`/`2`/`4` (PCM16 and the two G.711 variants) are
meaningful `encodeformat` values — AAC's `encodeformat=8` is unverified on E4702.

> **Browser pitfall — PCM16 / L16 is not web-encodable.** WebRTC browsers can only *send*
> Opus, G.711 A-law (PCMA), G.711 µ-law (PCMU) and G722. PCM16 (L16) and AAC are not in the
> browser's encoder set, so `codec=pcml` cannot be negotiated from a browser page — it only works
> with non-browser sources (ffmpeg, file, another pipeline). Keep `native=1` so G.711 is forwarded
> untouched; `native=0` decodes to PCM16 only for firmware that refuses G.711 (0x0E).

### Capability query (future work)

The SDK exposes `DH_DEVSTATE_TALK_ECTYPE` (`0x0009`) to query the device talk encoding type, which
would let go2rtc pick `encodeformat` automatically instead of defaulting to `1`. The demo does not
use it; this fork does not yet either.
