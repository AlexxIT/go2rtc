# Yandex

Source for receiving stream from new [Yandex IP camera](https://alice.yandex.ru/smart-home/security/ipcamera).

## Get Yandex token

1. Install HomeAssistant integration [YandexStation](https://github.com/AlexxIT/YandexStation).
2. Copy token from HomeAssistant config folder: `/config/.storage/core.config_entries`, key: `"x_token"`.

## Get device ID

1. Open this link in any browser: https://iot.quasar.yandex.ru/m/v3/user/devices
2. Copy ID of your camera, key: `"id"`.

## Configuration

```yaml
streams:
  yandex_stream: yandex:?x_token=XXXX&device_id=XXXX
  yandex_snapshot: yandex:?x_token=XXXX&device_id=XXXX&snapshot
  yandex_snapshot_custom_size: yandex:?x_token=XXXX&device_id=XXXX&snapshot=h=540
```
