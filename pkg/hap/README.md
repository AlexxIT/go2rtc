# Home Accessory Protocol

> PS. Character = Characteristic

**Device** - HomeKit end device (swith, camera, etc)

- mDNS name: `MyCamera._hap._tcp.local.`
- DeviceID - mac-like: `0E:AA:CE:2B:35:71`
- HomeKit device is described by:
  - one or more `Accessories` - has `AID` and `Services`  
  - `Services` - has `IID`, `Type` and `Characters`  
  - `Characters` - has `IID`, `Type`, `Format` and `Value`

**Client** - HomeKit client (iPhone, iPad, MacBook or opensource library)

- ClientID - static random UUID
- ClientPublic/ClientPrivate - static random 32 byte keypair
- can pair with Device (exchange ClientID/ClientPublic, ServerID/ServerPublic using Pin)
- can auth to Device using ClientPrivate
- holding persistant Secure connection to device
- can read device Accessories
- can read and write device Characters
- can subscribe on device Characters change (Event)

**Server** - HomeKit server (soft on end device or opensource library)

- ServerID - same as DeviceID (using for Client auth)
- ServerPublic/ServerPrivate - static random 32 byte keypair

## AAC ELD 

Requires ffmpeg built with `--enable-libfdk-aac`

```
-acodec libfdk_aac -aprofile aac_eld 
```

## Useful links

- https://github.com/apple/HomeKitADK/blob/master/Documentation/crypto.md
- https://github.com/apple/HomeKitADK/blob/master/HAP/HAPPairingPairSetup.c
- [Extracting HomeKit Pairing Keys](https://pvieito.com/2019/12/extract-homekit-pairing-keys)
- [HAP in AirPlay2 receiver](https://github.com/openairplay/airplay2-receiver/blob/master/ap2/pairing/hap.py)
- [HomeKit Secure Video Unofficial Specification](https://github.com/Supereg/secure-video-specification)
- [Homebridge Camera FFmpeg](https://sunoo.github.io/homebridge-camera-ffmpeg/configs/)