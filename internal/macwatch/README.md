# MAC Watch

Cameras get their IP address from a DHCP server. Usually the router tries to
keep the same address for the same device, but after a long power outage or
a router reset a camera can get a new address. Every link to this camera in
the go2rtc config will stop working.

This module remembers the MAC address of every camera, finds its actual IP
address and rewrites the config file when the address has changed.

## Config

```yaml
macwatch:
  interval: 60
  cameras:
    dahua1: aa:bb:cc:dd:ee:ff
    reolink1: 11-22-33-44-55-66
```

**cameras** - the MAC address for every stream name from the `streams` section.
Any format is supported: `aa:bb:cc:dd:ee:ff`, `AA-BB-CC-DD-EE-FF`,
`aabb.ccdd.eeff` and `aabbccddeeff`. Streams that use a hostname (ex.
`camera.local`) or a link to another stream are skipped.

**interval** - how often (in seconds) to check the IP addresses. Default is
`60`, `0` means check only once at startup.

**sweep** - send UDP probes to all hosts of the local subnet when the MAC
address is not found in the ARP table. Default is `true`. This is the only way
to find a new IP address of the camera, because the OS knows nothing about the
hosts it has never talked to.

**sweep_interval** - minimum interval (in seconds) between sweeps.
Default is `300`.

**verify** - check that the found IP address is alive on the port from the
stream link (ex. `554` for RTSP) before changing the config.
Default is `true`. The check is skipped for schemes with a dynamic
port (ex. `homekit`) or without a TCP port at all (ex. `xiaomi`).

**dry_run** - find and log the changes, but don't touch the config file.
Default is `false`.

## Example

The config contains an outdated IP address, the camera is actually
on `192.168.1.71`:

```yaml
streams:
  dahua1: rtsp://admin:password@192.168.1.64:554/cam/realmonitor?channel=1

macwatch:
  cameras:
    dahua1: aa:bb:cc:dd:ee:ff
```

The module will change it to:

```yaml
streams:
  dahua1: rtsp://admin:password@192.168.1.71:554/cam/realmonitor?channel=1
```

The config file is updated in place, so all comments and the rest of the file
will be kept as is. The running stream will be reconnected to the new address
without a restart of go2rtc.

## API

Current state of all cameras:

```text
GET /api/macwatch
```

Force a rescan and update the config:

```text
POST /api/macwatch
```

```json
{
  "enabled": true,
  "config_path": "/config/go2rtc.yaml",
  "interval": 60,
  "sweep": true,
  "verify": true,
  "dry_run": false,
  "cameras": [
    {
      "stream": "dahua1",
      "mac": "aa:bb:cc:dd:ee:ff",
      "source": "rtsp://admin:password@192.168.1.71:554/cam/realmonitor?channel=1",
      "ip": "192.168.1.71",
      "found": "192.168.1.71",
      "status": "updated",
      "updated": "2024-05-21T12:00:00+03:00"
    }
  ]
}
```

Discover the MAC addresses for all streams and get a ready to use config
section:

```text
GET /api/macwatch/discover
```

```json
{
  "cameras": { "dahua1": "aa:bb:cc:dd:ee:ff" },
  "yaml": "macwatch:\n  cameras:\n    dahua1: aa:bb:cc:dd:ee:ff\n"
}
```

It uses the current ARP table, so the camera must be online. The MAC address
will be found only for streams with an IP address in the source URL. Use the
result to create the `macwatch` section without searching MAC addresses
manually.

Possible statuses:

| Status        | Description                                            |
|---------------|--------------------------------------------------------|
| `ok`          | the MAC address matches the IP address from the config |
| `updated`     | the IP address has been changed in the config          |
| `dry_run`     | the IP address could be changed                        |
| `not_found`   | the MAC address is absent in the ARP table             |
| `unreachable` | the MAC address is found, but the host is not alive    |
| `static`      | the stream uses a hostname or a link to another stream |
| `no_stream`   | the stream is absent in go2rtc                         |
| `error`       | internal error, ex. config file is not writable        |

## Limitations

- The camera must be in the same subnet (L2 segment) as the go2rtc host.
  A MAC address of the host from another subnet is not visible from the local
  ARP table. If you use Docker, you must use "network host".

- Go2rtc must be started with a config file (`-c go2rtc.yaml`). The module
  can't change a config that was passed as a command line string.

- The stream name must be found in the config file. Streams from the second
  config file or from the Home Assistant integration are not supported yet.

- Some devices answer on two addresses with the same MAC address (for example
  a camera that is connected over both Wi-Fi bands). The module keeps the
  address that is already in the config as long as it is still in the ARP
  table, so it will not switch between them by itself. If that address
  disappears while the other one for the same MAC is still known, the module
  will use the other one after the port check.
