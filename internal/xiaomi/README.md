# Xiaomi

This source allows you to view cameras from the [Xiaomi Mi Home](https://home.mi.com/) ecosystem.

**Important:**

1. **Not all cameras are supported**. There are several P2P protocol vendors in the Xiaomi ecosystem. 
Currently, the **CS2** vendor is supported. However, the **TUTK** vendor is not supported.
2. Each time you connect to the camera, you need internet access to obtain encryption keys.
3. Connection to the camera is local only.

**Features:**

- Multiple Xiaomi accounts supported
- Cameras from multiple regions are supported for a single account
- Two-way audio is supported
- Cameras with multiple lenses are supported

## Setup

1. Goto go2rtc WebUI > Add > Xiaomi > Login with username and password
2. Receive verification code by email or phone if required.
3. Complete the captcha if required.
4. If everything is OK, your account will be added and you can load cameras from it.

**Example**

```yaml
xiaomi:
  1234567890: V1:***

streams:
  xiaomi1: 	xiaomi://1234567890:cn@192.168.1.123?did=9876543210&model=isa.camera.hlc7
```

## Configuration

You can change camera's quality: `subtype=hd/sd/auto`

```yaml
streams:
  xiaomi1: xiaomi://***&subtype=sd
```

You can use second channel for Dual cameras: `channel=1`

```yaml
streams:
  xiaomi1: xiaomi://***&channel=1
```
