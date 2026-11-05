# Experimental Wyze RDT / normal microSD retrieval

Tested on one unmodified Wyze Cam v3, firmware `4.36.16.6114`. This is an
interoperability contribution candidate, not a complete vendor RDT replacement.

## Layers and ownership

1. Use the existing authenticated native LAN session. This code performs no
   cloud login or device discovery through a cloud service.
2. HL10010 payload `04 01` enables the SID-scoped RDT worker; `04 02` disables it.
   HL10011 identifies media 4/result 1. The tested firmware responds with device
   protocol version 37, whereas the request builder uses version 5.
3. A separate DTLS channel 10 uses username `admin` and SHA256(ENR), all 32 bytes.
   AV's existing `AUTHPWD_admin` identity remains unchanged.
4. The RDT session performs symmetric HELLO/HELLO_ACK, ordered reliable DATA,
   direct and cumulative acknowledgements, bounded retries, and FIN/CLOSE.
5. Inside the RDT byte stream, a separate application protocol requests and
   validates a single normal recording. The caller owns the output writer and
   any partial prefix following an error.

The channel belongs to one operation. Probe and download close their secure
channel before disabling the worker. RDT-only adapter cancellation lets Pion
emit its DTLS close-notify and then unblock its reader. Reopening channel 10
within the same authenticated SID still failed on the tested firmware. The
library explicitly requires a fresh authenticated session rather than claiming
reuse works; the integration helper creates one fresh session per invocation.

Two additional cleanup changes are included. The initial IO-control request is
now sent synchronously before accepting even a buffered acknowledgement, and
the retry ticker cannot write after the call returns. Previously an already
buffered ACK could suppress the initial enable/disable send entirely. Concurrent
control transactions are rejected rather than consuming each other's queue.

On CC51 session teardown, the client sends a best-effort authenticated type-13
close notification before releasing its UDP socket. The exact firmware encoder
uses 48 bytes: magic uint32 `cc51`, bytes `02 13`, uint16 length 36, uint32 local
client record SID 0, direct path/reserved/ticket zero, uint32 Unix seconds,
eight-byte session RID, then HMAC-SHA1 over the first 28 bytes using UID plus
calculated auth key. The firmware matches both RID words, verifies role and
MAC, and closes only the matching established session. Three bounded sends are
unacknowledged; delivery is not proved. No legacy-IOTC close opcode is inferred.

## Transport facts

The 20-byte little-endian header contains magic `f1c2975a` (wire `5a97c2f1`),
type, version 5, uint16 payload length, uint32 sequence low/high, transmit count,
connection ID, and two auxiliary bytes. Default maximum payload is 1280.

Implemented types include HELLO `01`, DATA `02`, FIN `03`, CLOSE `04`, HELLO_ACK
`41`, DATA_ACK `42`, and cumulative ACK `46`. Connection ID zero is unspecified
on an already authenticated SID/channel; nonzero IDs must match the local
temporary ID. A strict nonzero-only check caused real zero-byte request failures:
the camera had accepted the request and was sending DATA/CACK with ID zero.
The receiver now handles that case while rejecting unrelated nonzero IDs.

Cumulative ACK names the highest contiguous sequence inclusively. The native
low sequence rolls over after `f0000000`, not ordinary uint64 wrap. Native
teardown uses FIN sequence words `ffffffff`; a clean peer CLOSE during local
shutdown is accepted, whereas a receive gap remains UnexpectedEOF.

## File application facts

The application header is five uint32 words: command, file size, starting byte
offset, payload length, and checksum. Request command 0 carries a one-byte ASCII
path length, path, zero resume offset, and zero rolling checksum.

SYNC command 1 has header `[1,0,0,5,0]` and five payload bytes: status plus file
size. Successful status 2 must advertise nonzero size. DATA command 16 uses
blocks up to 4096 bytes, sequential starting offsets, constant total size, and
a sum of signed int8 payload bytes modulo 2^32. This is not a cryptographic hash.

Firmware can queue DATA before SYNC. The reader streams validated blocks with
a fixed 4 KiB block buffer and requires exactly one successful, matching SYNC
plus the exact total byte count before success. Missing or contradictory SYNC,
invalid offsets, size changes, corrupt checksums, or short writes fail.

Only calendar-valid `/media/mmc/record/YYYYMMDD/HH/MM.mp4` and `_new.mp4` paths
are accepted. Times are camera local. The library caps files at 64 MiB; the
example/DWB adapter defaults to 16 MiB. Transfer deadline is 45 seconds, plus
bounded handshake/control and teardown. A missing/empty response is not treated
as a verified empty export. No remote storage writes are performed.

## Limits

- One camera/firmware tested; interoperability elsewhere is unproved.
- Before a camera restart, channel-10 startup became unavailable while live
  video and app microSD playback remained available. After the restart, twelve
  consecutive fresh-session requests through the proposed DWB adapter and the
  compiled applied go2rtc patches succeeded. Each body matched a previously
  decoded reference file exactly by SHA-256; temporary files and locks were
  released after each response. This establishes bounded repeatability on the
  tested firmware. It does not isolate the original stall's root cause or prove
  indefinite operation, recovery without restart, or cross-model reliability.
- Stop-and-wait sender, fixed receive window/buffer and retry policy. Full SACK,
  urgent streams, resume and directory enumeration are not implemented.
- No reliable parallel-download speedup has been established.
- Generic address/write synchronization prerequisites remain in this patch;
  maintainers may prefer extracting them with focused regressions and reconciling
  related upstream lifetime work.
- API placement, one-operation ownership, cross-project helper dependency and
  wider hardware coverage require maintainer review before merge.

## Provenance

Protocol facts came from static analysis of the official Wyze APK's RDT library
(API `4.3.4.5`) and the [official firmware archive](https://download.wyzecam.com/firmware/v3/demo_wcv3_4.36.16.6114.bin.zip),
then bounded live interoperability checks. Static analysis alone did not prove
runtime behavior. No vendor source, binaries or decompiled code are distributed.
Related [CuboAI playback work](https://github.com/Fredde87/cuboai-tutk/blob/main/cuboai_playback.py)
targets a different device and file protocol; no equivalence is assumed.
