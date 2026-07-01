# MPEG

This module provides an [HTTP API](../api/README.md) for:
 
- Streaming output in `mpegts` format.
- Streaming output in `adts` format.
- Streaming ingest in `mpegts` format.

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
