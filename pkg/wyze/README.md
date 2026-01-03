# Wyze

This source allows you to stream from [Wyze](https://wyze.com/) cameras using native P2P protocol without the Wyze app or SDK.

**Important:**

1. **Requires Wyze account**. You need to login once via the WebUI to load your cameras.
2. **Requires newer firmware with DTLS**. Only cameras with DTLS-enabled firmware are currently supported.
3. Internet access is only needed when loading cameras from your account. After that, all streaming is local P2P.
4. Connection to the camera is local only (direct P2P to camera IP).

**Features:**

- H.264 and H.265 video codec support
- AAC, G.711, PCM, and Opus audio codec support
- Two-way audio (intercom) support
- Resolution switching (HD/SD)

## Setup

1. Get your API Key from [Wyze Developer Portal](https://support.wyze.com/hc/en-us/articles/16129834216731)
2. Go to go2rtc WebUI > Add > Wyze
3. Enter your API ID, API Key, email, and password
4. Select cameras to add - stream URLs are generated automatically

**Example Config**

```yaml
wyze:
  user@email.com:
    api_id: "your-api-id"
    api_key: "your-api-key"
    password: "yourpassword"    # or MD5 triple-hash with "md5:" prefix

streams:
  wyze_cam: wyze://192.168.1.123?uid=WYZEUID1234567890AB&enr=xxx&mac=AABBCCDDEEFF
```

## Stream URL Format

The stream URL is automatically generated when you add cameras via the WebUI:

```
wyze://[IP]?uid=[P2P_ID]&enr=[ENR]&mac=[MAC]&dtls=true
```

| Parameter | Description |
|-----------|-------------|
| `IP` | Camera's local IP address |
| `uid` | P2P identifier (20 chars) |
| `enr` | Encryption key for DTLS |
| `mac` | Device MAC address |
| `dtls` | Enable DTLS encryption (default: true) |

## Configuration

### Resolution

You can change the camera's resolution using the `quality` parameter:

```yaml
streams:
  wyze_hd: wyze://...&quality=hd    # 1080P/2K (default)
  wyze_sd: wyze://...&quality=sd    # 360P
```

### Two-Way Audio

Two-way audio (intercom) is supported automatically. When a consumer sends audio to the stream, it will be transmitted to the camera's speaker.

## Supported Cameras

Cameras using the TUTK P2P protocol:

| Model | Name | Tested |
|-------|------|--------|
| WYZE_CAKP2JFUS | Wyze Cam v3 | |
| HL_CAM3P | Wyze Cam v3 Pro | |
| HL_CAM4 | Wyze Cam v4 | Yes |
| WYZECP1_JEF | Wyze Cam Pan | |
| HL_PANP | Wyze Cam Pan v2 | |
| HL_PAN3 | Wyze Cam Pan v3 | |
| WVOD1 | Wyze Video Doorbell | |
| WVOD2 | Wyze Video Doorbell v2 | |
| AN_RSCW | Wyze Video Doorbell Pro | |
| GW_BE1 | Wyze Cam Floodlight | |
| HL_WCO2 | Wyze Cam Outdoor | |
| HL_CFL2 | Wyze Cam Floodlight v2 | |
| LD_CFP | Wyze Battery Cam Pro | |
