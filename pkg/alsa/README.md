## Build

```shell
x86_64-linux-gnu-gcc -w -static asound_arch.c -o asound_amd64
i686-linux-gnu-gcc -w -static asound_arch.c -o asound_i386
aarch64-linux-gnu-gcc -w -static asound_arch.c -o asound_arm64
arm-linux-gnueabihf-gcc -w -static asound_arch.c -o asound_arm
mipsel-linux-gnu-gcc -w -static asound_arch.c -o asound_mipsle -D_TIME_BITS=32
```

## Useful links

- https://github.com/torvalds/linux/blob/master/include/uapi/sound/asound.h
- https://github.com/yobert/alsa
- https://github.com/Narsil/alsa-go
- https://github.com/alsa-project/alsa-lib
- https://github.com/anisse/alsa
- https://github.com/tinyalsa/tinyalsa

**Broken pipe**

- https://stackoverflow.com/questions/26545139/alsa-cannot-recovery-from-underrun-prepare-failed-broken-pipe
- https://klipspringer.avadeaux.net/alsa-broken-pipe-errors/
