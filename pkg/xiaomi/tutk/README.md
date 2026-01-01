# TUTK

The most terrible protocol I have ever had to work with.

## Messages

Ping from camera (24b). The shortest message.

```
off sample
0   0402      tutk magic
2   190a      tutk version (120a, 190a...)
4   0800      msg size = len(b)-16 = 24-16
6   0000      channel seq (always 0 for ping)
8   2804      msg type (2804 - ping from camera, 0804 - usual msg from camera)
10  1200      direction (12 - from camera, 21 - from client)
12  00000000  fixed
16  7ecc93c4  random
20  56c2561f  random
```

Usual msg from camera (52b + msg data).

```
off sample
12 e6e8      same bytes b[20:22]
14 0000      channel (0, 1, 5)
16 0c00      fixed
18 0000      fixed
20 e6e839da  random session id
24 66b0dc14  random session id
28 0070      command
30 0b00      version
32 0100      command seq
34 0000      ???
36 00000000  ???
40 00000000  ???
44 e300      msg data size
46 0000      ???
48 8f15a02f  random msg id
52 ...       msg data
```

Message with media from camera.

```
off sample
28  0c00      command
30  0b00      version
32  7700      command seq
34  0000      ??? data only for last message per pack (14/14)
36  0200      pack seq, don't know how packs used
38  0914      09/14 - message seq/messages per packs
40  01000000  fixed
42  0500      command 2
44  3200      command 2 seq
46  4f00      chunks count per this frame
48  1b00      chunk seq, starts from 0 (wrong for last chunk)
50  0004      frame data size
52  c8f6      random msg id
54  01000000  previous frame seq, starts from 0
58  02000000  current frame seq, starts from 1
```
