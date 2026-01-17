# TUTK/IOTC Protocol Reference for Wyze Cameras

This document provides a complete reverse-engineering reference for the ThroughTek TUTK/IOTC protocol as used by Wyze cameras. It covers the entire protocol stack from UDP transport through encrypted P2P streaming, enabling implementation of native Wyze camera streaming without the proprietary TUTK SDK.

## Table of Contents

1. [Protocol Stack Overview](#1-protocol-stack-overview)
2. [Encryption Layers](#2-encryption-layers)
3. [Connection Flow](#3-connection-flow)
4. [IOTC Packet Structures](#4-iotc-packet-structures)
5. [DTLS Transport](#5-dtls-transport)
6. [AV Login](#6-av-login)
7. [K-Command Authentication](#7-k-command-authentication)
8. [K-Command Control](#8-k-command-control)
9. [AV Frame Structure](#9-av-frame-structure)
10. [FRAMEINFO Structure](#10-frameinfo-structure)
11. [Codec Reference](#11-codec-reference)
12. [Two-Way Audio (Backchannel)](#12-two-way-audio-backchannel)
13. [Frame Reassembly](#13-frame-reassembly)
14. [Wyze Cloud API](#14-wyze-cloud-api)
15. [Cryptography Details](#15-cryptography-details)
16. [Constants Reference](#16-constants-reference)
17. [NEW Protocol (0xCC51) Overview](#17-new-protocol-0xcc51-overview)
18. [NEW Protocol Discovery](#18-new-protocol-discovery)
19. [NEW Protocol DTLS Wrapper](#19-new-protocol-dtls-wrapper)

---

## 1. Protocol Stack Overview

Wyze cameras support two protocol variants depending on firmware version:

| Protocol | Firmware | Magic | Discovery | Encryption |
|----------|----------|-------|-----------|------------|
| OLD | Cam v4 ≤ 4.52.9.4188 | TransCode | 0x0601/0x0602 | TransCode + DTLS |
| NEW | Cam v4 ≥ 4.52.9.5332 | 0xCC51 | 0x1002 | HMAC-SHA1 + DTLS |

### OLD Protocol Stack (TransCode-based)

```
┌─────────────────────────────────────────────────────────────┐
│                    Application Layer                        │
│         Video (H.264/H.265) + Audio (AAC/G.711/Opus)        │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│                    AV Frame Layer                           │
│    Frame Types, Channels, FRAMEINFO, Packet Reassembly      │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│                K-Command Authentication                     │
│      K10000-K10003 (XXTEA Challenge-Response)               │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│                    AV Login Layer                           │
│              Credentials + Capabilities Exchange            │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│                 DTLS 1.2 Encryption                         │
│      PSK = SHA256(ENR), ChaCha20-Poly1305 AEAD              │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│                    IOTC Session                             │
│         Discovery (0x0601) + Session Setup (0x0402)         │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│              TransCode Cipher ("Charlie")                   │
│            XOR + Bit Rotation Obfuscation                   │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│                      UDP Transport                          │
│                    Port 32761 (default)                     │
└─────────────────────────────────────────────────────────────┘
```

### NEW Protocol Stack (0xCC51-based)

```
┌─────────────────────────────────────────────────────────────┐
│                    Application Layer                        │
│         Video (H.264/H.265) + Audio (AAC/G.711/Opus)        │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│                    AV Frame Layer                           │
│    Frame Types, Channels, FRAMEINFO, Packet Reassembly      │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│                K-Command Authentication                     │
│      K10000-K10003 (XXTEA Challenge-Response)               │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│                    AV Login Layer                           │
│              Credentials + Capabilities Exchange            │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│                 DTLS 1.2 Encryption                         │
│      PSK = SHA256(ENR), ChaCha20-Poly1305 AEAD              │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│              NEW Protocol Wrapper (0xCC51)                  │
│    Discovery (0x1002) + DTLS Wrapper (0x1502) + HMAC-SHA1   │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│                      UDP Transport                          │
│                    Port 32761 (default)                     │
└─────────────────────────────────────────────────────────────┘
```

### Required Credentials

| Parameter | Description | Source |
|-----------|-------------|--------|
| UID | Device P2P identifier (20 chars) | Wyze Cloud API |
| ENR | Encryption key (16+ bytes) | Wyze Cloud API |
| MAC | Device MAC address | Wyze Cloud API |
| AuthKey | SHA256(ENR + MAC)[:6] in Base64 | Calculated |

### Credential Derivation

```
AuthKey = Base64(SHA256(ENR + uppercase(MAC))[0:6])
         with substitutions: '+' → 'Z', '/' → '9', '=' → 'A'

PSK = SHA256(ENR)  // 32 bytes for DTLS
```

---

## 2. Encryption Layers

The protocol uses three distinct encryption layers:

### Layer 1: TransCode ("Charlie" Cipher)

Applied to all IOTC Discovery and Session packets before UDP transmission.

**Algorithm:**
- XOR with magic string: `"Charlie is the designer of P2P!!"`
- 32-bit left rotation on each block
- Byte permutation/swapping

**When Applied:**
- Disco Request/Response (0x0601/0x0602)
- Session Request/Response (0x0402/0x0404)
- Data TX/RX wrappers (0x0407/0x0408)

### Layer 2: DTLS 1.2

Encrypts all data after session establishment.

| Parameter | Value |
|-----------|-------|
| Version | DTLS 1.2 |
| Cipher Suite | TLS_ECDHE_PSK_WITH_CHACHA20_POLY1305_SHA256 (0xCCAC) |
| PSK Identity | `AUTHPWD_admin` |
| PSK | SHA256(ENR) - 32 bytes |
| Curve | X25519 |

### Layer 3: XXTEA

Used for K-Command challenge-response authentication.

| Status | Key Derivation |
|--------|----------------|
| 1 (Default) | Key = `"FFFFFFFFFFFFFFFF"` (16 x 0xFF) |
| 3 (ENR16) | Key = ENR[0:16] |
| 6 (ENR32) | Double: decrypt with ENR[0:16], then with ENR[16:32] |

---

## 3. Connection Flow

### 3.1 OLD Protocol Flow (TransCode-based)

```
Client                                                 Camera
   │                                                      │
   │  ═══════════ Phase 1: IOTC Discovery ═══════════════ │
   │                                                      │
   │  Disco Stage 1 (0x0601, broadcast) ───────────────►  │
   │  ◄───────────────────────  Disco Response (0x0602)   │
   │  Disco Stage 2 (0x0601, direct) ──────────────────►  │
   │                                                      │
   │  ═══════════ Phase 2: IOTC Session ═════════════════ │
   │                                                      │
   │  Session Request (0x0402) ────────────────────────►  │
   │  ◄─────────────────────  Session Response (0x0404)   │
   │                                                      │
   │  ═══════════ Phase 3: DTLS Handshake ═══════════════ │
   │                                                      │
   │  ClientHello (in DATA_TX 0x0407) ─────────────────►  │
   │  ◄─────────────────────  ServerHello + KeyExchange   │
   │  ClientKeyExchange + Finished ────────────────────►  │
   │  ◄─────────────────────────────────  DTLS Finished   │
   │                                                      │
   │  ═══════════ Phase 4: AV Login ═════════════════════ │
   │                                                      │
   │  AV Login #1 (magic=0x0000) ──────────────────────►  │
   │  AV Login #2 (magic=0x2000) ──────────────────────►  │
   │  ◄─────────────────────  AV Login Response (0x2100)  │
   │  ACK (0x0009) ────────────────────────────────────►  │
   │                                                      │
   │  ═══════════ Phase 5: K-Authentication ═════════════ │
   │                                                      │
   │  K10000 (Auth Request) ───────────────────────────►  │
   │  ◄─────────────────────────  K10001 (Challenge 16B)  │
   │  ACK (0x0009) ────────────────────────────────────►  │
   │  K10002 (Response 38B) ───────────────────────────►  │
   │  ◄─────────────────────────  K10003 (Result, JSON)   │
   │  ACK (0x0009) ────────────────────────────────────►  │
   │                                                      │
   │  ═══════════ Phase 6: Streaming ════════════════════ │
   │                                                      │
   │  ◄───────────────────────────────  Video/Audio Data  │
   │  ◄───────────────────────────────  Video/Audio Data  │
   │                       ...                            │
```

### 3.2 NEW Protocol Flow (0xCC51-based)

```
Client                                                 Camera
   │                                                      │
   │  ═══════════ Phase 1: Discovery (0x1002) ═══════════ │
   │                                                      │
   │  seq=0, ticket=0 (broadcast) ────────────────────►   │
   │  ◄───────────────  seq=1, ticket=T (response)        │
   │  seq=2, ticket=T (echo) ─────────────────────────►   │
   │  ◄─────────────────────────────  seq=3, ticket=T     │
   │                                                      │
   │  ═══════════ Phase 2: DTLS Handshake (0x1502) ══════ │
   │                                                      │
   │  ClientHello (wrapped in 0x1502) ────────────────►   │
   │  ◄─────────────────────  ServerHello + KeyExchange   │
   │  ClientKeyExchange + Finished ───────────────────►   │
   │  ◄─────────────────────────────────  DTLS Finished   │
   │                                                      │
   │  ═══════════ Phase 3: AV Login ═════════════════════ │
   │                                                      │
   │  AV Login #1 (magic=0x0000) ──────────────────────►  │
   │  AV Login #2 (magic=0x2000) ──────────────────────►  │
   │  ◄─────────────────────  AV Login Response (0x2100)  │
   │  ACK (0x0009) ────────────────────────────────────►  │
   │                                                      │
   │  ═══════════ Phase 4: K-Authentication ═════════════ │
   │                                                      │
   │  K10000 (Auth Request) ───────────────────────────►  │
   │  ◄─────────────────────────  K10001 (Challenge 16B)  │
   │  ACK (0x0009) ────────────────────────────────────►  │
   │  K10002 (Response 38B) ───────────────────────────►  │
   │  ◄─────────────────────────  K10003 (Result, JSON)   │
   │  ACK (0x0009) ────────────────────────────────────►  │
   │                                                      │
   │  ═══════════ Phase 5: Streaming ════════════════════ │
   │                                                      │
   │  ◄───────────────────────────────  Video/Audio Data  │
   │  ◄───────────────────────────────  Video/Audio Data  │
   │                       ...                            │
```

**Key Differences from OLD Protocol:**
- Discovery uses 4-packet handshake (seq 0→1→2→3) instead of 2-stage discovery + session setup
- No TransCode encryption layer - packets use HMAC-SHA1 authentication instead
- DTLS records wrapped in 0x1502 frames with auth bytes appended

---

## 4. IOTC Packet Structures

### 4.1 IOTC Frame Header (16 bytes)

All IOTC packets share this outer wrapper:

```
Offset  Size  Field        Description
──────────────────────────────────────────────────────────────
[0]     1     Marker1      Always 0x04
[1]     1     Marker2      Always 0x02
[2]     1     Marker3      Always 0x1A
[3]     1     Mode         0x02 (Disco), 0x0A (Session), 0x0B (Data)
[4-5]   2     BodySize     Body length in bytes (LE)
[6-7]   2     Sequence     Packet sequence number (LE)
[8-9]   2     Command      Command ID (LE)
[10-11] 2     Flags        Command-specific flags (LE)
[12-15] 4     RandomID     Random identifier or metadata
```

### 4.2 Disco Request (0x0601) - 80 bytes total

```
Offset  Size  Field        Description
──────────────────────────────────────────────────────────────
[0-15]  16    Header       IOTC Frame Header (cmd=0x0601)
[16-35] 20    UID          Device UID (null-padded ASCII)
[36-51] 16    Reserved     Zero-filled
[52-59] 8     RandomID     8 random bytes for session
[60]    1     Stage        1=broadcast, 2=direct
[61-71] 11    Reserved     Zero-filled
[72-79] 8     AuthKey      Calculated auth key
```

### 4.3 Session Request (0x0402) - 52 bytes total

```
Offset  Size  Field        Description
──────────────────────────────────────────────────────────────
[0-15]  16    Header       IOTC Frame Header (cmd=0x0402)
[16-35] 20    UID          Device UID (null-padded ASCII)
[36-43] 8     RandomID     Same as Disco
[44-47] 4     Reserved     Zero-filled
[48-51] 4     Timestamp    Unix timestamp (LE)
```

### 4.4 Data TX (0x0407) - Variable

Wraps DTLS records for transmission:

```
Offset  Size  Field        Description
──────────────────────────────────────────────────────────────
[0-15]  16    Header       IOTC Frame Header (cmd=0x0407)
[16-17] 2     RandomID[0:2]
[18]    1     Channel      0=Main (DTLS client), 1=Back (DTLS server)
[19]    1     Marker       Always 0x01
[20-23] 4     Const        Always 0x0000000C
[24-31] 8     RandomID     Full 8-byte random ID
[32+]   var   Payload      DTLS record data
```

---

## 5. DTLS Transport

DTLS records are wrapped in IOTC DATA_TX (0x0407) packets for transmission and extracted from DATA_RX (0x0408) packets on reception.

### PSK Callback

```
Identity: "AUTHPWD_admin"
PSK: SHA256(ENR_string) → variable length (see below)
```

#### PSK Length Determination

**CRITICAL**: The TUTK SDK treats the binary PSK as a NULL-terminated C string.
This means the effective PSK length is determined by the first `0x00` byte in the SHA256 hash:

```
hash = SHA256(ENR)
psk_length = position of first 0x00 byte in hash (or 32 if no 0x00)
psk = hash[0:psk_length] + zeros[psk_length:32]
```

**Example 1** - No NULL byte in hash (full 32-byte PSK):
```
ENR: "aKzdqckqZ8HUHFe5"
SHA256: 3e5b96b8d6fc7264b531e1633de9526929d453cb47606c55d574a6e0ef5eb95f
        ^^ No 0x00 byte → PSK length = 32
```

**Example 2** - NULL byte at position 11 (11-byte PSK):
```
ENR: "GkB9S7cX38GgzSC6"
SHA256: 16549c533b4e9812808f91|00|95f6edf00365266f09ea1e0328df3eee1ce127ed
                            ^^ 0x00 at position 11 → PSK length = 11
PSK:    16549c533b4e9812808f91000000000000000000000000000000000000000000
```

### Nonce Construction

```
nonce[12] = IV[12] XOR (epoch[2] || sequenceNumber[6] || padding[4])
```

### AEAD Additional Data

```
additional_data = epoch[2] || sequenceNumber[6] || contentType[1] || version[2] || payloadLength[2]
```

---

## 6. AV Login

After DTLS handshake, two login packets establish the AV session.

### AV Login Packet #1 (570 bytes)

```
Offset    Size  Field          Value/Description
──────────────────────────────────────────────────────────────
[0-1]     2     Magic          0x0000 (LE)
[2-3]     2     Version        0x000C (12)
[4-15]    12    Reserved       Zero-filled
[16-17]   2     PayloadSize    0x0222 (546)
[18-19]   2     Flags          0x0001
[20-23]   4     RandomID       4 random bytes
[24-279]  256   Username       "admin" (null-padded)
[280-535] 256   Password       ENR string (null-padded)
[536-539] 4     Resend         0=disabled, 1=enabled (see 9.6)
[540-543] 4     SecurityMode   0x00000002 (AV_SECURITY_AUTO)
[544-547] 4     AuthType       0x00000000 (PASSWORD)
[548-551] 4     SyncRecvData   0x00000000
[552-555] 4     Capabilities   0x001F07FB
[556-569] 14    Reserved       Zero-filled
```

### AV Login Packet #2 (572 bytes)

Same structure as #1 with:
- Magic = 0x2000
- PayloadSize = 0x0224 (548)
- Flags = 0x0000
- RandomID[0] incremented by 1

### AV Login Response (0x2100)

```
Offset  Size  Field            Description
──────────────────────────────────────────────────────────────
[0-1]   2     Magic            0x2100
[2-3]   2     Version          0x000C
[4]     1     ResponseType     0x10 = success
[5-15]  11    Reserved
[16-19] 4     PayloadSize      0x00000024 (36)
[20-23] 4     Checksum         Echo from request
[24-27] 4     Reserved
[28]    1     Flag1
[29]    1     EnableFlag       0x01 if enabled
[30]    1     Flag2
[31]    1     TwoWayAudio      0x01 if intercom supported
[32-35] 4     Reserved
[36-39] 4     BufferConfig     0x00000004
[40-43] 4     Capabilities     0x001F07FB
[44-57] 14    Reserved
```

---

## 7. K-Command Authentication

K-Commands use the "HL" header format and are sent inside IOCTRL frames.

### IOCTRL Frame Wrapper (40+ bytes)

```
Offset  Size  Field          Description
──────────────────────────────────────────────────────────────
[0-1]   2     Magic          0x000C
[2-3]   2     Version        0x000C
[4-7]   4     AVSeq          AV sequence number (LE)
[8-15]  8     Reserved       Zero-filled
[16-17] 2     IOCTRLMagic    0x7000
[18-19] 2     SubChannel     Command sequence (increments)
[20-23] 4     IOCTRLSeq      Always 0x00000001
[24-27] 4     PayloadSize    HL payload size + 4
[28-31] 4     Flag           Matches SubChannel
[32-35] 4     Reserved
[36-37] 2     IOType         0x0100
[38-39] 2     Reserved
[40+]   var   HLPayload      K-Command data
```

### HL Header (16 bytes)

```
Offset  Size  Field          Description
──────────────────────────────────────────────────────────────
[0-1]   2     Magic          "HL" (0x48 0x4C)
[2]     1     Version        5
[3]     1     Reserved       0x00
[4-5]   2     CommandID      10000, 10001, 10002, etc. (LE)
[6-7]   2     PayloadLen     Payload length after header (LE)
[8-15]  8     Reserved       Zero-filled
[16+]   var   Payload        Command-specific data
```

### K10000 - Auth Request (16 + JSON bytes)

```
Offset  Size  Field          Description
──────────────────────────────────────────────────────────────
[0-15]  16    HLHeader       CommandID = 10000, PayloadLen = len(JSON)
[16+]   var   JSONPayload    Audio codec preferences
```

**JSON Payload:**
```json
{"cameraInfo":{"audioEncoderList":[137,138,140]}}
```

Where audioEncoderList contains supported codec IDs: 137=PCMU, 138=PCMA, 140=PCM.

### K10001 - Challenge (33+ bytes)

```
Offset  Size  Field          Description
──────────────────────────────────────────────────────────────
[0-15]  16    HLHeader       CommandID = 10001
[16]    1     Status         Key selection: 1, 3, or 6
[17-32] 16    Challenge      XXTEA-encrypted challenge bytes
```

**Status Interpretation:**
| Status | Key Source |
|--------|------------|
| 1 | Default key: 16 x 0xFF |
| 3 | ENR[0:16] |
| 6 | Double decrypt: first ENR[0:16], then ENR[16:32] |

### K10002 - Challenge Response (38 bytes)

```
Offset  Size  Field          Description
──────────────────────────────────────────────────────────────
[0-15]  16    HLHeader       CommandID = 10002, PayloadLen = 22
[16-31] 16    Response       XXTEA-decrypted challenge
[32-35] 4     SessionID      Random 4-byte session identifier
[36]    1     VideoFlag      1 = enable video stream
[37]    1     AudioFlag      1 = enable audio stream
```

### K10003 - Auth Result

Variable length, contains JSON payload:

```json
{
  "connectionRes": "1",
  "cameraInfo": {
    "basicInfo": {
      "firmware": "4.52.9.4188",
      "mac": "AABBCCDDEEFF",
      "model": "HL_CAM4"
    },
    "channelResquestResult": {
      "audio": "1",
      "video": "1"
    }
  }
}
```

After K10003, video/audio streaming begins automatically.

---

## 8. K-Command Control

### K10010 - Control Channel (18 bytes)

Start or stop media streams:

```
Offset  Size  Field          Description
──────────────────────────────────────────────────────────────
[0-15]  16    HLHeader       CommandID = 10010, PayloadLen = 2
[16]    1     MediaType      1=Video, 2=Audio, 3=ReturnAudio
[17]    1     Enable         1=Enable, 2=Disable
```

**Media Types:**
| Value | Type | Description |
|-------|------|-------------|
| 1 | Video | Main video stream |
| 2 | Audio | Audio from camera |
| 3 | ReturnAudio | Intercom (audio to camera) |
| 4 | RDT | Raw data transfer |

### K10056 - Set Resolution (21 bytes)

```
Offset  Size  Field          Description
──────────────────────────────────────────────────────────────
[0-15]  16    HLHeader       CommandID = 10056, PayloadLen = 5
[16]    1     FrameSize      Resolution + 1 (see table)
[17-18] 2     Bitrate        KB/s value (LE)
[19-20] 2     FPS            Frames per second, 0 = auto
```

**Frame Sizes:**
| Value | Resolution |
|-------|------------|
| 1 | 1080P (1920x1080) |
| 2 | 360P (640x360) |
| 3 | 720P (1280x720) |
| 4 | 2K (2560x1440) |

**Bitrate Values:**
| Value | Rate |
|-------|------|
| 0xF0 (240) | Maximum |
| 0x3C (60) | SD quality |

### K10052 - Set Resolution Doorbell (22 bytes)

Used by doorbell models (WYZEDB3, WVOD1, HL_WCO2, WYZEC1) instead of K10056:

```
Offset  Size  Field          Description
──────────────────────────────────────────────────────────────
[0-15]  16    HLHeader       CommandID = 10052, PayloadLen = 6
[16-17] 2     Bitrate        KB/s value (LE)
[18]    1     FrameSize      Resolution + 1 (see table above)
[19]    1     FPS            Frames per second, 0 = auto
[20-21] 2     Reserved       Zero-filled
```

**Note:** K10052 has a different field order than K10056 (bitrate before frameSize).

---

## 9. AV Frame Structure

### 9.1 Channels

| Value | Name | Description |
|-------|------|-------------|
| 0x03 | Audio | Audio frames (always single-packet) |
| 0x05 | I-Video | Keyframes (can be multi-packet) |
| 0x07 | P-Video | Predictive frames (can be multi-packet) |

### 9.2 Frame Types

| Type | Name | Header Size | Has FRAMEINFO |
|------|------|-------------|---------------|
| 0x00 | Cont | 28 bytes | No |
| 0x01 | EndSingle | 28 bytes | Yes (40B) |
| 0x04 | ContAlt | 28 bytes | No |
| 0x05 | EndMulti | 28 bytes | Yes (40B) |
| 0x08 | Start | 36 bytes | No |
| 0x09 | StartAlt | 36 bytes | Yes if pkt_total=1 |
| 0x0D | EndExt | 36 bytes | Yes (40B) |

### 9.3 28-Byte Header Layout

Used by: Cont (0x00), EndSingle (0x01), ContAlt (0x04), EndMulti (0x05)

```
Offset  Size  Field              Description
──────────────────────────────────────────────────────────────
[0]     1     Channel            0x03/0x05/0x07
[1]     1     FrameType          0x00/0x01/0x04/0x05
[2-3]   2     Version            0x000B (11)
[4-5]   2     TxSequence         Global incrementing sequence (LE)
[6-7]   2     Magic              0x507E ("P~")
[8]     1     Channel            Duplicate of [0]
[9]     1     StreamIndex        0x00 normal, 0x01 for End packets
[10-11] 2     PacketCounter      Running counter (does NOT reset per frame)
[12-13] 2     pkt_total          Total packets in this frame (LE)
[14-15] 2     pkt_idx/Marker     Packet index OR 0x0028 = FRAMEINFO present
[16-17] 2     PayloadSize        Payload bytes (LE)
[18-19] 2     Reserved           0x0000
[20-23] 4     PrevFrameNo        Previous frame number (LE)
[24-27] 4     FrameNo            Current frame number (LE) → USE FOR REASSEMBLY
```

### 9.4 36-Byte Header Layout

Used by: Start (0x08), StartAlt (0x09), EndExt (0x0D)

```
Offset  Size  Field              Description
──────────────────────────────────────────────────────────────
[0]     1     Channel            0x03/0x05/0x07
[1]     1     FrameType          0x08/0x09/0x0D
[2-3]   2     Version            0x000B (11)
[4-5]   2     TxSequence         Global incrementing sequence (LE)
[6-7]   2     Magic              0x507E ("P~")
[8-11]  4     TimestampOrID      Variable (not reliable)
[12-15] 4     Flags              Variable
[16]    1     Channel            Duplicate of [0]
[17]    1     StreamIndex        0x00 normal, 0x01 for End/Audio
[18-19] 2     ChannelFrameIdx    Per-channel index (NOT for reassembly)
[20-21] 2     pkt_total          Total packets in this frame (LE)
[22-23] 2     pkt_idx/Marker     Packet index OR 0x0028 = FRAMEINFO present
[24-25] 2     PayloadSize        Payload bytes (LE)
[26-27] 2     Reserved           0x0000
[28-31] 4     PrevFrameNo        Previous frame number (LE)
[32-35] 4     FrameNo            Current frame number (LE) → USE FOR REASSEMBLY
```

### 9.5 FRAMEINFO Marker (0x0028)

The value at offset [14-15] (28-byte) or [22-23] (36-byte) has dual meaning:

| Condition | Interpretation |
|-----------|----------------|
| End packet AND value == 0x0028 | FRAMEINFO present (40 bytes at payload end) |
| Otherwise | Actual packet index within frame |

**Note:** 0x0028 hex = 40 decimal. For non-End packets, this could be pkt_idx=40.

### 9.6 Resend Mode

The `resend` field in the AV Login packet (offset [536-539]) controls the packet format used for streaming. Setting this value determines whether retransmission support is enabled:

#### resend=0: Direct Format (Simpler)

```
[channel][frameType][version 2B][seq 2B]...[payload]
```

Example:
```
0000: 05 00 0b 00 6d 00 81 4e 05 00 63 00 86 00 00 00
      ^^ ^^
      |  frameType=0x00 (continuation)
      channel=0x05 (I-Video)
```

**Characteristics:**
- First byte is channel: 0x03=Audio, 0x05=I-Video, 0x07=P-Video
- No 0x0c wrapper overhead
- No Frame Index packets (1080 bytes)
- Simpler parsing, less bandwidth

#### resend=1: Wrapped Format (With Resend Support)

```
[0x0c][variant][version 2B][seq 2B]...[channel at offset 16/24]
```

Example:
```
0000: 0c 05 0b 00 e4 00 64 00 0a 00 00 14 01 00 00 00
      ^^ ^^
      |  variant=0x05
      0x0c wrapper (resend marker)
0010: 07 01 c8 00 01 00 28 00 ...
      ^^
      channel=0x07 (P-Video) at offset 16
```

**Characteristics:**
- First byte is always 0x0c (resend wrapper)
- Channel byte at offset 16 (variant < 0x08) or 24 (variant >= 0x08)
- Additional 1080-byte Frame Index packets sent periodically
- Enables packet retransmission for reliable delivery

#### Header Size Rule

| Variant | Header Size | Channel Offset |
|---------|-------------|----------------|
| < 0x08 | 36 bytes | 16 |
| >= 0x08 | 44 bytes | 24 |

### 9.7 Frame Index Packets (Inner Byte 0x0c)

When using `resend=1`, the camera sends periodic **Frame Index** packets (also called Resend Buffer Status).

#### Packet Structure (1080 bytes total)

```
OUTER HEADER (16 bytes):
0000: 0c 00 0b 00 [seq 2B] [sub 2B] [counter 2B] 14 14 01 00 00 00
      ^^^^                                       ^^^^^
      cmd=0x0c                                   magic

INNER HEADER (20 bytes):
0010: 0c 00 00 00 00 00 00 00 14 04 00 00 00 00 00 00 00 00 00 00
      ^^^^                   ^^^^^
      inner cmd              payload_size = 0x0414 = 1044 bytes

PAYLOAD DATA (starting at offset 0x20):
0020: 00 00 00 00          // 4 zero bytes
0024: [ch] [ft]            // channel + frame type
0026: [data 2B] [data 2B]  // varies by packet type
...
0030: [prev_frame 4B LE]   // previous frame number
0034: [curr_frame 4B LE]   // current frame number
```

#### Key Offsets

| Offset | Size | Field |
|--------|------|-------|
| 0x24 (36) | 1 | Channel (0x05=I-Video, 0x07=P-Video) |
| 0x25 (37) | 1 | Frame type |
| 0x30 (48) | 4 | Previous frame number (LE) |
| 0x34 (52) | 4 | Current frame number (LE) |

#### Packet Types

| Channel | Description |
|---------|-------------|
| 0x05 | I-Video Frame Index - consecutive frame numbers for GOP sync |
| 0x07 | P-Video - buffer window status (oldest/newest buffered frame) |

---

## 10. FRAMEINFO Structure

### 10.1 RX FRAMEINFO (40 bytes) - From Camera

Appended to the end of End packets (0x01, 0x05, 0x0D, or 0x09 when pkt_total=1):

```
Offset  Size  Field          Description
──────────────────────────────────────────────────────────────
[0-1]   2     codec_id       Video: 0x4E (H.264), 0x50 (H.265)
                             Audio: 0x90 (AAC), 0x89 (G.711μ), etc.
[2]     1     flags          Video: 0x00=P-frame, 0x01=I-frame (keyframe)
                             Audio: (sr_idx << 2) | (bits16 << 1) | stereo
[3]     1     cam_index      Camera index (usually 0)
[4]     1     online_num     Number of viewers
[5]     1     framerate      FPS (e.g., 20, 30)
[6]     1     frame_size     0=1080P, 1=SD, 2=360P, 4=2K
[7]     1     bitrate        Bitrate value
[8-11]  4     timestamp_us   Microseconds within second (0-999999)
[12-15] 4     timestamp      Unix timestamp in seconds (LE)
[16-19] 4     payload_size   Total payload size for validation (LE)
[20-23] 4     frame_no       Absolute frame counter (LE)
[24-39] 16    device_id      MAC address as ASCII + padding
```

### 10.2 TX FRAMEINFO (16 bytes) - To Camera

Used for audio backchannel (intercom):

```
Offset  Size  Field          Description
──────────────────────────────────────────────────────────────
[0-1]   2     codec_id       0x0090 (AAC Wyze), 0x0089 (G.711μ), etc.
[2]     1     flags          (sr_idx << 2) | (bits16 << 1) | stereo
[3]     1     cam_index      0
[4]     1     online_num     1 (for TX)
[5]     1     tags           0
[6-11]  6     reserved       Zero-filled
[12-15] 4     timestamp_ms   Cumulative: (frame_no - 1) * frame_duration_ms
```

### 10.3 Audio Flags Encoding

```
flags = (sample_rate_index << 2) | (bits16 << 1) | stereo

Example: 16kHz, 16-bit, Mono
  sr_idx=3, bits16=1, stereo=0
  flags = (3 << 2) | (1 << 1) | 0 = 0x0E
```

---

## 11. Codec Reference

### 11.1 Video Codecs

| ID (Hex) | ID (Dec) | Name |
|----------|----------|------|
| 0x4C | 76 | MPEG-4 |
| 0x4D | 77 | H.263 |
| 0x4E | 78 | H.264/AVC |
| 0x4F | 79 | MJPEG |
| 0x50 | 80 | H.265/HEVC |

### 11.2 Audio Codecs

| ID (Hex) | ID (Dec) | Name |
|----------|----------|------|
| 0x86 | 134 | AAC Raw |
| 0x87 | 135 | AAC ADTS |
| 0x88 | 136 | AAC LATM |
| 0x89 | 137 | G.711 μ-law (PCMU) |
| 0x8A | 138 | G.711 A-law (PCMA) |
| 0x8B | 139 | ADPCM |
| 0x8C | 140 | PCM 16-bit LE |
| 0x8D | 141 | Speex |
| 0x8E | 142 | MP3 |
| 0x8F | 143 | G.726 |
| 0x90 | 144 | AAC Wyze |
| 0x92 | 146 | Opus |

### 11.3 Sample Rate Index

| Index | Frequency |
|-------|-----------|
| 0x00 | 8000 Hz |
| 0x01 | 11025 Hz |
| 0x02 | 12000 Hz |
| 0x03 | 16000 Hz |
| 0x04 | 22050 Hz |
| 0x05 | 24000 Hz |
| 0x06 | 32000 Hz |
| 0x07 | 44100 Hz |
| 0x08 | 48000 Hz |

---

## 12. Two-Way Audio (Backchannel)

### 12.1 Activation Flow

1. Send K10010 with MediaType=3 (ReturnAudio), Enable=1
2. Wait for K10011 response confirming activation
3. Camera initiates DTLS connection back (we become DTLS **server**)
4. Use Channel 1 (IOTCChannelBack) for audio transmission

### 12.2 Audio TX Frame Format

All audio TX uses 0x09 single-packet frames with 36-byte header:

```
Offset  Size  Field              Description
──────────────────────────────────────────────────────────────
[0]     1     Channel            0x03 (Audio)
[1]     1     FrameType          0x09 (StartAlt/Single)
[2-3]   2     Version            0x000C (12)
[4-7]   4     TxSeq              Audio TX sequence number (LE)
[8-11]  4     TimestampUS        Timestamp in microseconds (LE)
[12-15] 4     Flags              0x00000001 (first), 0x00100001 (subsequent)
[16]    1     Channel            0x03
[17]    1     FrameType          0x01 (EndSingle)
[18-19] 2     PrevFrameNo        prev_frame_no (16-bit, LE)
[20-21] 2     pkt_total          0x0001 (always single packet)
[22-23] 2     Flags              0x0010
[24-27] 4     PayloadSize        audio_len + 16 (includes FRAMEINFO)
[28-31] 4     PrevFrameNo        prev_frame_no (32-bit, LE)
[32-35] 4     FrameNo            Current frame number (LE)
[36...]       AudioPayload       AAC/G.711/Opus data
[end-16] 16   FRAMEINFO          TX FRAMEINFO (16 bytes)
```

---

## 13. Frame Reassembly

### Algorithm

```
1. Parse packet header to extract:
   - channel, frameType, pkt_idx, pkt_total, frame_no

2. Detect frame transition:
   - If frame_no changed from previous packet:
     - Emit previous frame if complete
     - Log incomplete frames

3. Store packet data:
   - Key: pkt_idx (0 to pkt_total-1)
   - Value: payload bytes (COPY - buffer is reused!)

4. Store FRAMEINFO if present:
   - Only in End packets (0x01, 0x05, 0x0D)
   - Or 0x09 when pkt_total == 1

5. Check completion:
   - All pkt_total packets received?
   - FRAMEINFO present?

6. Assemble frame:
   - Concatenate: packets[0] + packets[1] + ... + packets[pkt_total-1]
   - Validate size against FRAMEINFO.payload_size
   - Emit to consumer
```

### Example: Multi-Packet I-Frame (14 packets)

```
Packet 1:  ch=0x05 type=0x08 pkt=0/14 frame=1   ← Start (36B header)
Packet 2:  ch=0x05 type=0x00 pkt=1/14 frame=1   ← Cont (28B header)
Packet 3:  ch=0x05 type=0x00 pkt=2/14 frame=1   ← Cont
...
Packet 13: ch=0x05 type=0x00 pkt=12/14 frame=1  ← Cont
Packet 14: ch=0x05 type=0x05 pkt=13/14 frame=1  ← EndMulti + FRAMEINFO
```

### Example: Single-Packet P-Frame

```
Packet 1:  ch=0x07 type=0x01 pkt=0/1 frame=42   ← EndSingle + FRAMEINFO
```

---

## 14. Wyze Cloud API

### 14.1 Authentication

**Endpoint:** `POST https://auth-prod.api.wyze.com/api/user/login`

**Password Hashing:** Triple MD5
```
hash = password
for i in range(3):
    hash = MD5(hash).hex()
```

**Request Headers:**
```
Content-Type: application/json
X-API-Key: WMXHYf79Nr5gIlt3r0r7p9Tcw5bvs6BB4U8O8nGJ
Phone-Id: <random-uuid>
User-Agent: wyze_ios_2.50.0
```

**Request Body:**
```json
{
  "email": "user@example.com",
  "password": "<triple-md5-hash>"
}
```

**Response:**
```json
{
  "access_token": "...",
  "refresh_token": "...",
  "user_id": "..."
}
```

### 14.2 Device List

**Endpoint:** `POST https://api.wyzecam.com/app/v2/home_page/get_object_list`

**Request Body:**
```json
{
  "access_token": "<token>",
  "phone_id": "<id>",
  "app_name": "com.hualai.WyzeCam",
  "app_ver": "com.hualai.WyzeCam___2.50.0",
  "app_version": "2.50.0",
  "phone_system_type": 1,
  "sc": "9f275790cab94a72bd206c8876429f3c",
  "sv": "9d74946e652647e9b6c9d59326aef104",
  "ts": <unix_millis>
}
```

**Response (filtered for cameras):**
```json
{
  "device_list": [
    {
      "mac": "AABBCCDDEEFF",
      "p2p_id": "HSBJYB5HSETGCDWD111A",
      "enr": "roTRg3tiuL3TjXhm...",
      "ip": "192.168.1.100",
      "nickname": "Front Door",
      "product_model": "HL_CAM4",
      "dtls": 1,
      "firmware_ver": "4.52.9.4188"
    }
  ]
}
```

---

## 15. Cryptography Details

### 15.1 XXTEA Algorithm

Block cipher used for K-Auth challenge-response:

```
Constants:
  DELTA = 0x9E3779B9

Function mx(sum, y, z, p, e, k):
  return (((z >> 5) ^ (y << 2)) + ((y >> 3) ^ (z << 4))) ^
         ((sum ^ y) + (k[(p & 3) ^ e] ^ z))

Decrypt(data, key):
  v = data as uint32[] (little-endian)
  k = key as uint32[]
  n = len(v)
  rounds = 6 + 52/n
  sum = rounds * DELTA

  for round in range(rounds):
    e = (sum >> 2) & 3
    for p in range(n-1, 0, -1):
      z = v[p-1]
      v[p] -= mx(sum, y=v[(p+1) mod n], z, p, e, k)
      y = v[p]
    z = v[n-1]
    v[0] -= mx(sum, y=v[1], z, 0, e, k)
    y = v[0]
    sum -= DELTA

  return v as bytes
```

### 15.2 TransCode ("Charlie" Cipher)

Obfuscation cipher for IOTC packets:

```
Magic string: "Charlie is the designer of P2P!!"

Process in 16-byte blocks:
  1. XOR each byte with corresponding position in magic string
  2. Treat as 4 x uint32, rotate left by varying amounts
  3. Apply byte permutation pattern

Permutation for 16-byte block:
  [11, 9, 8, 15, 13, 10, 12, 14, 2, 1, 5, 0, 6, 4, 7, 3]
```

### 15.3 AuthKey Calculation

```
input = ENR + uppercase(MAC)
hash = SHA256(input)
raw = hash[0:6]
b64 = Base64Encode(raw)
authkey = b64.replace('+', 'Z').replace('/', '9').replace('=', 'A')
```

---

## 16. Constants Reference

### 16.1 IOTC Commands

| Command | Value | Description |
|---------|-------|-------------|
| CmdDiscoReq | 0x0601 | Discovery request |
| CmdDiscoRes | 0x0602 | Discovery response |
| CmdSessionReq | 0x0402 | Session request |
| CmdSessionRes | 0x0404 | Session response |
| CmdDataTX | 0x0407 | Data transmission |
| CmdDataRX | 0x0408 | Data reception |
| CmdKeepaliveReq | 0x0427 | Keepalive request |
| CmdKeepaliveRes | 0x0428 | Keepalive response |

### 16.2 Magic Values

| Magic | Value | Description |
|-------|-------|-------------|
| MagicAVLogin1 | 0x0000 | AV Login packet 1 |
| MagicAVLogin2 | 0x2000 | AV Login packet 2 |
| MagicAVLoginResp | 0x2100 | AV Login response |
| MagicIOCtrl | 0x7000 | IOCTRL frame |
| MagicChannelMsg | 0x1000 | Channel message |
| MagicACK | 0x0009 | ACK frame |

### 16.3 K-Commands

| Command | ID | Description |
|---------|-----|-------------|
| KCmdAuth | 10000 | Auth request (with JSON) |
| KCmdChallenge | 10001 | Challenge from camera |
| KCmdChallengeResp | 10002 | Challenge response |
| KCmdAuthResult | 10003 | Auth result (JSON) |
| KCmdControlChannel | 10010 | Start/stop media |
| KCmdControlChannelResp | 10011 | Control response |
| KCmdSetResolutionDB | 10052 | Set resolution (doorbell) |
| KCmdSetResolutionDBResp | 10053 | Resolution response (doorbell) |
| KCmdSetResolution | 10056 | Set resolution/bitrate |
| KCmdSetResolutionResp | 10057 | Resolution response |

### 16.4 IOTYPE Values

| Type | Value | Description |
|------|-------|-------------|
| IOTypeVideoStart | 0x01FF | Start video |
| IOTypeVideoStop | 0x02FF | Stop video |
| IOTypeAudioStart | 0x0300 | Start audio |
| IOTypeAudioStop | 0x0301 | Stop audio |
| IOTypeSpeakerStart | 0x0350 | Start intercom |
| IOTypeSpeakerStop | 0x0351 | Stop intercom |
| IOTypeDevInfoReq | 0x0340 | Device info request |
| IOTypeDevInfoRes | 0x0341 | Device info response |
| IOTypePTZCommand | 0x1001 | PTZ control |
| IOTypeReceiveFirstFrame | 0x1002 | Request keyframe |

### 16.5 Protocol Constants

| Constant | Value | Description |
|----------|-------|-------------|
| DefaultPort | 32761 | TUTK discovery port |
| ProtocolVersion | 0x000C | Version 12 |
| DefaultCapabilities | 0x001F07FB | Standard caps |
| MaxPacketSize | 2048 | Max UDP packet |
| IOTCChannelMain | 0 | Main channel (DTLS client) |
| IOTCChannelBack | 1 | Backchannel (DTLS server) |

### 16.6 NEW Protocol Constants

| Constant | Value | Description |
|----------|-------|-------------|
| MagicNewProto | 0xCC51 | NEW protocol magic (LE) |
| CmdNewProtoDiscovery | 0x1002 | Discovery command |
| CmdNewProtoDTLS | 0x1502 | DTLS data command |
| NewProtoPayloadSize | 0x0028 | 40 bytes payload |
| NewProtoPacketSize | 52 | Total discovery packet size |
| NewProtoHeaderSize | 28 | DTLS packet header size |
| NewProtoAuthSize | 20 | Auth bytes (HMAC-SHA1) |

---

## 17. NEW Protocol (0xCC51) Overview

The NEW protocol (magic 0xCC51) is used by Wyze Cam v4 with firmware 4.52.9.5332 and later. It replaces the TransCode cipher layer with HMAC-SHA1 authentication and simplifies the discovery process.

### Key Differences from OLD Protocol

| Aspect | OLD Protocol | NEW Protocol |
|--------|--------------|--------------|
| Magic | TransCode encoded | 0xCC51 |
| Discovery | 0x0601/0x0602 + 0x0402/0x0404 | 0x1002 (4-packet handshake) |
| Encryption | TransCode + DTLS | HMAC-SHA1 + DTLS |
| DTLS Wrapper | DATA_TX 0x0407 | 0x1502 with auth bytes |
| P2P Servers | Required for relay | Not required (LAN only) |

### Authentication

All NEW protocol packets include a 20-byte HMAC-SHA1 authentication field:

```go
// Key derivation
authKey := CalculateAuthKey(enr, mac)  // 8-byte key from ENR + MAC
key := append([]byte(uid), authKey...) // UID (20 bytes) + AuthKey (8 bytes)

// HMAC-SHA1 calculation
h := hmac.New(sha1.New, key)
h.Write(packetHeader)  // Header bytes before auth field
authBytes := h.Sum(nil) // 20 bytes
```

---

## 18. NEW Protocol Discovery

Discovery uses command 0x1002 with a 4-packet handshake sequence.

### 18.1 Discovery Packet Structure (52 bytes)

```
Offset  Size  Field          Description
──────────────────────────────────────────────────────────────
[0-1]   2     Magic          0xCC51 (little-endian)
[2-3]   2     Flags          0x0000 (constant)
[4-5]   2     Command        0x1002 (Discovery)
[6-7]   2     PayloadSize    0x0028 (40 bytes)
[8-9]   2     Direction      0x0000=Request, 0xFFFF=Response
[10-11] 2     Reserved       0x0000
[12-13] 2     Sequence       0, 1, 2, or 3
[14-15] 2     Ticket         0x0000 initially, then from camera
[16-23] 8     SessionID      Random[2] + Constant[6]
[24-31] 8     Capabilities   0x00 08 03 04 1d 00 00 00
[32-51] 20    AuthBytes      HMAC-SHA1(key, header[0:32])
```

### 18.2 Handshake Sequence

```
Step  Direction    Seq  Ticket  Description
────────────────────────────────────────────────────────────────
1     Client→Cam   0    0x0000  Discovery request (broadcast)
2     Cam→Client   1    T       Discovery response (ticket assigned)
3     Client→Cam   2    T       Echo request (confirms ticket)
4     Cam→Client   3    T       Echo ACK (handshake complete)
```

### 18.3 SessionID Generation

```go
sessionID := make([]byte, 8)
rand.Read(sessionID[:2])                              // Random prefix
copy(sessionID[2:], []byte{0x76, 0x0a, 0x9d, 0x24, 0x88, 0xba}) // Constant suffix
```

---

## 19. NEW Protocol DTLS Wrapper

After discovery, DTLS records are wrapped in command 0x1502 frames with HMAC-SHA1 authentication.

### 19.1 DTLS Wrapper Structure (variable size)

```
Offset  Size  Field          Description
──────────────────────────────────────────────────────────────
[0-1]   2     Magic          0xCC51 (little-endian)
[2-3]   2     Flags          0x0000
[4-5]   2     Command        0x1502 (DTLS)
[6-7]   2     PayloadSize    16 + dtls_len + 20
[8-9]   2     Direction      0x0000=Request
[10-11] 2     Reserved       0x0000
[12-13] 2     Sequence       0x0010 (fixed for DTLS)
[14-15] 2     Ticket         From discovery handshake
[16-23] 8     SessionID      8 bytes from discovery
[24-27] 4     Channel        1=Main (client), 2=Back (server)
[28-N]  var   DTLSPayload    Raw DTLS record
[N:N+20] 20   AuthBytes      HMAC-SHA1(key, bytes[0:N])
```

### 19.2 PayloadSize Calculation

```
PayloadSize = 16 + len(DTLSPayload) + 20

Where:
  16 = seq(2) + ticket(2) + sessionID(8) + channel(4)
  20 = AuthBytes (HMAC-SHA1)
```

### 19.3 TX/RX Processing

**Transmit (TX):**
1. Build header with magic, command, payload size
2. Append session fields (seq, ticket, sessionID, channel)
3. Append DTLS payload
4. Calculate HMAC-SHA1 over entire packet (excluding auth bytes position)
5. Append auth bytes

**Receive (RX):**
1. Verify magic == 0xCC51
2. Extract DTLS payload from position 28 to (length - 20)
3. Strip 20 auth bytes from end
4. Pass DTLS payload to DTLS layer
