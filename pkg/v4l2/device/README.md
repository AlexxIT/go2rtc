Build on Ubuntu

```bash
sudo apt install gcc-x86-64-linux-gnu
sudo apt install gcc-i686-linux-gnu
sudo apt install gcc-aarch64-linux-gnu binutils
sudo apt install gcc-arm-linux-gnueabihf
sudo apt install gcc-mipsel-linux-gnu

x86_64-linux-gnu-gcc -w -static arch.c -o arch_x86_64
i686-linux-gnu-gcc -w -static arch.c -o arch_i686
aarch64-linux-gnu-gcc -w -static arch.c -o arch_aarch64
arm-linux-gnueabihf-gcc -w -static arch.c -o arch_armhf
mipsel-linux-gnu-gcc -static arch.c -o arch_mipsel
```

## Useful links

- https://github.com/torvalds/linux/blob/master/include/uapi/linux/videodev2.h
