# Video For Linux Two

Build on Ubuntu

```bash
sudo apt install gcc-x86-64-linux-gnu
sudo apt install gcc-i686-linux-gnu
sudo apt install gcc-aarch64-linux-gnu binutils
sudo apt install gcc-arm-linux-gnueabihf
sudo apt install gcc-mipsel-linux-gnu

x86_64-linux-gnu-gcc -w -static videodev2_arch.c -o videodev2_x86_64
i686-linux-gnu-gcc -w -static videodev2_arch.c -o videodev2_i686
aarch64-linux-gnu-gcc -w -static videodev2_arch.c -o videodev2_aarch64
arm-linux-gnueabihf-gcc -w -static videodev2_arch.c -o videodev2_armhf
mipsel-linux-gnu-gcc -w -static videodev2_arch.c -o videodev2_mipsel -D_TIME_BITS=32
```

## Useful links

- https://github.com/torvalds/linux/blob/master/include/uapi/linux/videodev2.h
