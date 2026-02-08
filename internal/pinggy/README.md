# Pinggy

[Pinggy](https://pinggy.io/) - nice service for public tunnels to your local services.

**Features:**

- A free account does not require registration.
- It does not require downloading third-party binaries and works over the SSH protocol.
- Works with HTTP, TCP and UDP protocols.
- Creates HTTPS for your HTTP services.

> [!IMPORTANT]
> A free account creates a tunnel with a random address that only works for an hour. It is suitable for testing purposes ONLY.

> [!CAUTION]
> Public access to go2rtc without authorization puts your entire home network at risk. Use with caution.

**Why:**

- It's easy to set up HTTPS for testing two-way audio.
- It's easy to check whether external access via WebRTC technology will work.
- It's easy to share direct access to your RTSP or HTTP camera with the go2rtc developer. If such access is necessary to debug your problem.

## Configuration

You will find public links in the go2rtc log after startup.

**Tunnel to go2rtc WebUI.**

```yaml
pinggy:
  tunnel: http://localhost:1984
```

**Tunnel to RTSP camera.**

For example, you have camera: `rtsp://admin:password@192.168.1.123/cam/realmonitor?channel=1&subtype=0`

```yaml
pinggy:
  tunnel: tcp://192.168.10.91:554
```

In go2rtc logs you will get similar output:

```
16:17:43.167 INF [pinggy] proxy url=tcp://abcde-123-123-123-123.a.free.pinggy.link:12345
```

Now you have a working stream:

```
rtsp://admin:password@abcde-123-123-123-123.a.free.pinggy.link:12345/cam/realmonitor?channel=1&subtype=0
```
