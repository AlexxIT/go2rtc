## Profiles

- Profile A - For access control configuration
- Profile C - For door control and event management
- Profile S - For basic video streaming
  - Video streaming and configuration
- Profile T - For advanced video streaming
  - H.264 / H.265 video compression
  - Imaging settings
  - Motion alarm and tampering events
  - Metadata streaming
  - Bi-directional audio

## Services

https://www.onvif.org/profiles/specifications/

- https://www.onvif.org/ver10/device/wsdl/devicemgmt.wsdl
- https://www.onvif.org/ver20/imaging/wsdl/imaging.wsdl
- https://www.onvif.org/ver10/media/wsdl/media.wsdl

## TMP

|                        | Dahua   | Reolink | TP-Link |
|------------------------|---------|---------|---------|
| GetCapabilities        | no auth | no auth | no auth |
| GetServices            | no auth | no auth | no auth |
| GetServiceCapabilities | no auth | no auth | auth    |
| GetSystemDateAndTime   | no auth | no auth | no auth |
| GetNetworkInterfaces   | auth    | auth    | auth    |
| GetDeviceInformation   | auth    | auth    | auth    |
| GetProfiles            | auth    | auth    | auth    |
| GetScopes              | auth    | auth    | auth    |

- Dahua - onvif://192.168.10.90:80
- Reolink - onvif://192.168.10.92:8000
- TP-Link - onvif://192.168.10.91:2020/onvif/device_service
- 