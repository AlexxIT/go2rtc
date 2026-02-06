# MPEG-TS

This module provides an [HTTP API](../api/README.md) for:
 
- Streaming output in `mpegts` format.
- Streaming output in `adts` format.
- Streaming ingest in `mpegts` format.

> [!NOTE]
> This module is probably better called mpeg. Because AAC is part of MPEG-2 and MPEG-4 and MPEG-TS is part of MPEG-2.

## MPEG-TS Server

```shell
ffplay http://localhost:1984/api/stream.ts?src=camera1
```

## ADTS Server

```shell
ffplay http://localhost:1984/api/stream.aac?src=camera1
```

## Streaming ingest

```shell
ffmpeg -re -i BigBuckBunny.mp4 -c copy -f mpegts http://localhost:1984/api/stream.ts?dst=camera1
```
