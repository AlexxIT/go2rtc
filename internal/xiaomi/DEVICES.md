# Xiaomi known cameras

## Supported

| name                                          | year | model                                          | proto             | codecs     | quality             |
|-----------------------------------------------|------|------------------------------------------------|-------------------|------------|---------------------|
| Xiaomi Smart Camera PTZ Version               | 2018 | chuangmi.camera.ipc009                         | ?                 |            |                     |
| Xiaomi Smart Camera PTZ Version               | 2019 | chuangmi.camera.ipc019 / MJSXJ05CM             | cs2+tcp           | hevc, pcma |                     |
| Mi 360 Home Security Camera 2K                | 2020 | chuangmi.camera.029a02 / MJSXJ09C / BHR4457GL  | сs2               | hevc, pcma |                     |
| Mi 360 Home Security Camera 2K Pro            | 2020 | chuangmi.camera.021a04 / MJSXJ06CM / BHR4193GL | сs2               | hevc, pcma |                     |
| Mi Home Security Camera 1080p                 | 2020 | isa.camera.hlc6                                | cs2+udp           | hevc, pcma | 1920x1080, 640x360  |
| Xiaomi Smart Camera PTZ Version 2K            | 2020 | chuangmi.camera.ip029a                         | cs2+udp           | hevc, pcma |                     |
| Xiaomi Smart Doorbell 2                       | 2020 | madv.cateye.miowlv2                            | cs2               |            |                     |
| IMILAB Security Camera EC3 Pro                | 2021 | chuangmi.camera.042a02                         | cs2               | hevc, pcma |                     |
| IMILAB EC5 Floodlight Camera 2K               | 2021 | chuangmi.camera.055a02 / CMSXJ55A              | сs2               |            |                     |
| Mi Camera 2K                                  | 2021 | isa.camera.hlc7 / MJSXJ03HL / BHR5255GL        | cs2+udp           | hevc, pcma | 2304x1296*, 640x360 |
| Xiaomi Smart Camera PTZ Version SE+           | 2021 | chuangmi.camera.026c02                         | cs2               |            |                     |
| Xiaomi Outdoor Camera CW400                   | 2022 | isa.camera.hlc8 / MJSXJ04HL                    | cs2               | hevc, opus |                     |
| Xiaomi Smart Camera 2 AI Enhanced Edition     | 2022 | chuangmi.camera.051a01                         | cs2               | hevc, opus |                     |
| Xiaomi Smart Camera C300                      | 2022 | xiaomi.camera.c01a01                           | cs2               |            |                     |
| Xiaomi Smart Doorbell 1S                      | 2022 | loock.cateye.v06                               | cs2               | hevc, pcma |                     |
| IMILAB EC5 Floodlight Camera 2К               | 2023 | chuangmi.camera.055c02                         | сs2               |            |                     |
| Xiaomi Outdoor Camera AW300                   | 2023 | mxiang.camera.mod13                            | cs2               | hevc, opus |                     |
| Xiaomi Smart Camera 2 PTZ Version             | 2023 | chuangmi.camera.039c01                         | cs2+tcp           | hevc, opus |                     |
| Xiaomi Smart Camera 3 PTZ Version             | 2023 | chuangmi.camera.069a01                         | cs2+tcp           | hevc, opus |                     |
| Xiaomi Smart Camera C200                      | 2023 | chuangmi.camera.046c04                         | cs2+tcp           | hevc, opus | 1920x1080*, 640x360 |
| Xiaomi Outdoor Camera CW500                   | 2024 | isa.camera.cw500                               | cs2+tcp           | hevc, opus |                     |
| Xiaomi Outdoor Camera CW500 Dual-Lens Edition | 2024 | isa.camera.hlmax                               | cs2+udp           | hevc, opus |                     |
| Xiaomi Outdoor Camera CW700S                  | 2024 | isa.camera.700sa                               | cs2               |            |                     |
| Xiaomi Smart Camera C300 Dual (CN)            | 2024 | chuangmi.camera.72ac1                          | cs2+tcp           | hevc, opus | 2304x1296*, 848x480 |
| Xiaomi Smart Camera C301                      | 2024 | mxiang.camera.c301                             | cs2               |            |                     |
| Xiaomi Smart Camera C400                      | 2024 | chuangmi.camera.039c04                         | cs2               |            |                     |
| Xiaomi Smart Camera C500 Pro                  | 2024 | chuangmi.camera.061a03                         | cs2               | hevc, opus |                     |
| Xiaomi Smart Camera C700                      | 2024 | chuangmi.camera.81ac1                          | cs2               | hevc, opus |                     |
| Xiaomi Smart camera/doorbell (PH300)          | 2024 | midr.cateye.ph300                              | cs2+tcp / cs2+udp | hevc, opus | 720x1280            |
| Xiaomi Outdoor Camera CW100 Dual              | 2025 | chuangmi.camera.075ae1                         | cs2               | hevc, opus |                     |
| Xiaomi Smart Camera 4 Dual Camera Edition     | 2025 | xiaomi.camera.082ac1                           | cs2               | hevc, opus |                     |
| Xiaomi Smart Camera 4 Zoom Edition            | 2025 | xiaomi.camera.083ac1                           | cs2+tcp           | hevc, opus |                     |
| Xiaomi Smart Camera C100                      | 2025 | chuangmi.camera.077ac1                         | cs2+tcp           | hevc, opus | 2304x1296*, 848x480 |
| Xiaomi Smart Camera C701                      | 2025 | chuangmi.camera.079ae2                         | cs2               | hevc, opus |                     |
| Xiaomi Smart Camera 3 3K                      | 2026 | xiaomi.camera.069a03                           | cs2+tcp           | hevc, opus | 2960x1666*, 848x480 |

- `chuangmi.camera.046c04`, `chuangmi.camera.72ac1` - Have HD quality on `subtype=3`
- `isa.camera.hlc7` - Starts streaming with SD quality, causing H265 SPS to have incorrect data
- `xiaomi.camera.069a03` - `subtype=1` is 848x480; default and `subtype=3` are 2960x1666
- `chuangmi.camera.077ac1` - default is 848x480; `subtype=3` (or `0`) is 2304x1296

## Limited support

| name                                      | year | model                              | format        | version              | codecs     | quality                      |
| ----------------------------------------- | ---- | ---------------------------------- | ------------- | -------------------- | ---------- | ---------------------------- |
| Mijia Smart Camera 1080P                  | 2017 | mijia.camera.v1                    | xiaomi/miss   | TUTK/25 SDK 3.3.0.6  | h264, pcma | 1920x1080, 640x360           |
| Mijia Smart Camera PTZ Version (fw 3.3.6) | 2017 | chuangmi.camera.v2                 | xiaomi/legacy | TUTK/18              | h264, pcma | 1280x720, 640x360            |
| Mijia Smart Camera PTZ Version (fw 4.0.9) | 2017 | chuangmi.camera.v2                 | xiaomi/miss   | TUTK/25 SDK 3.3.0.6  | h264, pcma | 1280x720, 640x360            |
| Mijia Xiaobai Smart Camera                | 2017 | chuangmi.camera.xiaobai            | xiaomi/legacy | TUTK/18              | h264, pcma | 1920x1080, 640x360           |
| Xiaomi Dafang 1080p (fw 5.5.1_1025)       | 2017 | isa.camera.df3                     | xiaomi/miss   | TUTK/18 SDK 3.0.1.50 | h264, pcma | 1920x1080, 640x360           |
| Xiaomi Dafang 1080p (fw 5.5.1.194)        | 2017 | isa.camera.df3                     | xiaomi/legacy | TUTK/16              | h264, pcml | 1920x1080                    |
| Xiaofang Smart Camera                     | 2017 | isa.camera.isc5                    | xiaomi/legacy | TUTK/16              | h264, pcml | 1920x1080                    |
| IMILAB Home Security Camera Basic         | 2018 | chuangmi.camera.ipc016             | xiaomi/miss   | TUTK/25 SDK 3.3.0.4  | hevc, pcma |                              |
| Xiaofang Smart Camera                     | 2018 | isa.camera.isc5c1                  | xiaomi/miss   | TUTK/18              | h264, pcma | 1920x1080, 640x360           |
| Xiaomi Mijia Smart Camera                 | 2018 | mijia.camera.v3                    | xiaomi/miss   | TUTK/25 SDK 3.3.0.5  | h264, pcma | 1920x1080, 640x360           |
| Aqara Smart Camera G2                     | 2019 | lumi.camera.gwagl01                | xiaomi/legacy | TUTK/18              | hevc       | 1920x1080, 1280x720, 640x360 |
| IMILAB Home Security Camera A1 (EU)       | 2019 | chuangmi.camera.ipc019e / CMSXJ19E | xiaomi/legacy | TUTK/18              | hevc, pcma | 2304x1296, 640x360           |
| Xiaomi Smart Camera PTZ Version           | 2019 | chuangmi.camera.ipc019             | xiaomi/miss   | TUTK/25 SDK 3.3.0.4  | hevc, pcma | 1920x1080, 640x360           |
| Xiaomi Smart Doorbell                     | 2019 | loock.cateye.v02                   | xiaomi/miss   | TUTK/20              | hevc, pcma | 1280x720, 640x360            |

- `isa.camera.isc5` - doesn't support quality
- `chuangmi.camera.v2` and `isa.camera.df3` - supported by both the old and new protocols

## Not supported

| name                                         | year | model                  | format        | version               | codecs     | quality    |
|----------------------------------------------|------|------------------------|---------------|-----------------------|------------|------------|
| Xiaomi Loock CatY                            | 2018 | loock.cateye.v01       | xiaomi/legacy | TUTK/20 SDK 3.1.5.147 |            |            |
| Xiaomi XiaoBai Smart Camera Enhanced Edition | 2018 | chuangmi.camera.v6     |               |                       |            |            |
| IMI Home Security Camera 1080P               | 2019 | chuangmi.camera.ipc017 | xiaomi/legacy | TUTK/18 SDK 3.0.1.50  | hevc, pcma | 1920x1080* |

## Useful links

- https://mijia.wiki/
- https://home.miot-spec.com/
