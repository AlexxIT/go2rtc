With the ngrok integration, you can get external access to your streams when your Internet connection is behind a private IP address.

- you may need external access for two different things:
    - WebRTC streams (tunnel the WebRTC TCP port, e.g. 8555)
    - go2rtc web interface (tunnel the API HTTP port, e.g. 1984)
- ngrok supports authorization for your web interface
- ngrok automatically adds HTTPS to your web interface

The ngrok free subscription has the following limitations:

- You can reserve a free domain for serving the web interface, but the TCP address you get will always be random and will change with each restart of the ngrok agent (not a problem for WebRTC streams)
- You can forward multiple ports from a single agent, but you can only run one ngrok agent on the free plan

go2rtc will automatically get your external TCP address (if you enable it in the ngrok config) and use it for WebRTC connections (if you enable it in the WebRTC config).

You need to manually download the [ngrok agent](https://ngrok.com/download) for your OS and register with the [ngrok service](https://ngrok.com/signup).

**Tunnel for only WebRTC Stream**

You need to add your [ngrok authtoken](https://dashboard.ngrok.com/get-started/your-authtoken) and WebRTC TCP port to YAML:

```yaml
ngrok:
  command: ngrok tcp 8555 --authtoken eW91IHNoYWxsIG5vdCBwYXNzCnlvdSBzaGFsbCBub3QgcGFzcw
```

**Tunnel for WebRTC and Web interface**

You need to create `ngrok.yaml` config file and add it to the go2rtc config:

```yaml
ngrok:
  command: ngrok start --all --config ngrok.yaml
```

ngrok config example:

```yaml
version: "2"
authtoken: eW91IHNoYWxsIG5vdCBwYXNzCnlvdSBzaGFsbCBub3QgcGFzcw
tunnels:
  api:
    addr: 1984  # use the same port as in the go2rtc config
    proto: http
    basic_auth:
      - admin:password  # you can set login/pass for your web interface
  webrtc:
    addr: 8555  # use the same port as in the go2rtc config
    proto: tcp
```

See the [ngrok agent documentation](https://ngrok.com/docs/agent/config/) for more details on the ngrok configuration file.
