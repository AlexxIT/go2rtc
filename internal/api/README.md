# HTTP API

The HTTP API is the main part for interacting with the application. Default address: `http://localhost:1984/`.

The HTTP API is described in [OpenAPI](../../website/api/openapi.yaml) format. It can be explored in [interactive viewer](https://go2rtc.org/api/). WebSocket API described [here](ws/README.md).

The project's static HTML and JS files are located in the [www](../../www) folder. An external developer can use them as a basis for integrating go2rtc into their project or for developing a custom web interface for go2rtc.

The contents of `www` folder are built into go2rtc when building, but you can use configuration to specify an external folder as the source of static files.

## Configuration

**Important!** go2rtc passes requests from localhost and Unix sockets without HTTP authorization, even if you have it configured. It is your responsibility to set up secure external access to the API. If not properly configured, an attacker can gain access to your cameras and even your server.

- you can disable HTTP API with `listen: ""` and use, for example, only RTSP client/server protocol
- you can enable HTTP API only on localhost with `listen: "127.0.0.1:1984"` setting
- you can change the API `base_path` and host go2rtc on your main app webserver suburl
- all files from `static_dir` hosted on root path: `/`
- you can use raw TLS cert/key content or path to files

```yaml
api:
  listen: ":1984"    # default ":1984", HTTP API port ("" - disabled)
  username: "admin"  # default "", Basic auth for WebUI
  password: "pass"   # default "", Basic auth for WebUI
  local_auth: true   # default false, Enable auth check for localhost requests
  base_path: "/rtc"  # default "", API prefix for serving on suburl (/api => /rtc/api)
  static_dir: "www"  # default "", folder for static files (custom web interface)
  origin: "*"        # default "", allow CORS requests (only * supported)
  tls_listen: ":443" # default "", enable HTTPS server
  tls_cert: |        # default "", PEM-encoded fullchain certificate for HTTPS
    -----BEGIN CERTIFICATE-----
    ...
    -----END CERTIFICATE-----
  tls_key: |         # default "", PEM-encoded private key for HTTPS
    -----BEGIN PRIVATE KEY-----
    ...
    -----END PRIVATE KEY-----
  unix_listen: "/tmp/go2rtc.sock"  # default "", unix socket listener for API
```

**PS:**

- MJPEG over WebSocket plays better than native MJPEG because Chrome [bug](https://bugs.chromium.org/p/chromium/issues/detail?id=527446)
- MP4 over WebSocket was created only for Apple iOS because it doesn't support file streaming
