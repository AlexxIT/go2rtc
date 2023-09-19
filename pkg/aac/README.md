## AAC-LD and AAC-ELD

| Codec   | Rate  | QuickTime | ffmpeg | VLC |
|---------|-------|-----------|--------|-----|
| AAC-LD  | 8000  | yes       | no     | no  |
| AAC-LD  | 16000 | yes       | no     | no  |
| AAC-LD  | 22050 | yes       | yes    | no  |
| AAC-LD  | 24000 | yes       | yes    | no  |
| AAC-LD  | 32000 | yes       | yes    | no  |
| AAC-ELD | 8000  | yes       | no     | no  |
| AAC-ELD | 16000 | yes       | no     | no  |
| AAC-ELD | 22050 | yes       | yes    | yes |
| AAC-ELD | 24000 | yes       | yes    | yes |
| AAC-ELD | 32000 | yes       | yes    | yes |

## Useful links

- [4.6.20 Enhanced Low Delay Codec](https://csclub.uwaterloo.ca/~ehashman/ISO14496-3-2009.pdf)
- https://stackoverflow.com/questions/40014508/aac-adts-for-aacobject-eld-packets
- https://code.videolan.org/videolan/vlc/-/blob/master/modules/packetizer/mpeg4audio.c
