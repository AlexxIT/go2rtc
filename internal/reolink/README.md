# Reolink

Experimental native Baichuan source for Reolink cameras on a trusted local network.

- H.264/H.265 video from `main`, `sub`, and `extern` profiles
- AAC audio or camera ADPCM converted to PCMA
- PCMA/PCMU/PCM/PCML two-way audio converted to camera ADPCM
- Direct LAN TCP or UID-discovered LAN-P2P over reliable UDP
- Explicit NVR and multi-lens channels

No FFmpeg, RTSP, or HTTP-FLV input bridge is required.

## Compatibility

The adapter discovers the selected profile's codecs from live media and probes talkback directly. Model identity, observed channels, and capability-query status are diagnostic; they do not select a model-specific media path.

Project hardware testing covers:

- E1 Zoom/E340 `main` and `sub` over TCP
- dual-lens TrackFlex `main` and `sub` on both channels over TCP and UID/LAN-P2P
- H.264, H.265, AAC, and talkback across those tests

Current Reolink specifications place [E1 Outdoor Pro](https://reolink.com/us/product/e1-outdoor-pro/), [Duo 3 PoE](https://support.reolink.com/c/duo-3-poe/), and [TrackMix PoE](https://reolink.com/us/product/reolink-trackmix-poe/) inside the same codec, profile, and talkback envelope. Other likely compatibility candidates include Duo 2 WiFi, TrackMix WiFi 6, and RLC-1212A. These are compatibility candidates, not tested support claims for this adapter.

### Community test reports

- **Video Doorbell PoE (D340P), firmware `3.0.0.7106_2609241883`.** Main and extern video, camera audio, and talkback worked. [Report](https://github.com/AlexxIT/go2rtc/pull/2413#issuecomment-6862976195)
- **RLC-823S2 and Video Doorbell PoE.** Main and extern worked on the RLC-823S2, and sub worked on two doorbells, with audio and talkback. The reporter saw Intel VAAPI detection crashes with FFmpeg 8.0 and reports stable detection with software decoding. [Report](https://github.com/AlexxIT/go2rtc/pull/2413#issuecomment-7095339748)
- **Elite Pro Floodlight PoE (F760P), firmware `v3.2.0.5932_2601201959`.** H.265 main video worked. Talkback worked from the H.264 sub view. [Report](https://github.com/AlexxIT/go2rtc/pull/2413#issuecomment-6862976195)
- **RLC-833A, RLC-811A, RLC-510WA, E1 Pro, and Video Doorbell PoE.** Video, camera audio, and talkback worked over one connection per camera. This report used go2rtc 1.9.14 with PR commit `3b7f07b` and two additional talkback patches. [Report](https://github.com/AlexxIT/go2rtc/pull/2413#issuecomment-6727321058)
- **Duo 2 PoE and RLC-811A.** An early build rejected larger login replies. The reporter confirmed the login error cleared after updating to the build with a higher reply limit. [Affected cameras](https://github.com/AlexxIT/go2rtc/pull/2413#issuecomment-6177457020) · [Retest](https://github.com/AlexxIT/go2rtc/pull/2413#issuecomment-6320310020)

### Intel VAAPI detection

This update improves H.264 startup and recovery. A retest on the affected Intel setup is pending. For a VAAPI surface-sync error, see the reporting details under [Diagnostics](#diagnostics).

## Configuration

```yaml
streams:
  camera_main: reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@camera.local/main?audio=0&backchannel=0
  camera_sub: reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@camera.local/sub?audio=1&backchannel=1
  camera_channel: reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@camera.local/main?channel=1
  camera_uid: reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@${REOLINK_UID}/main?transport=uid
  camera_uid_routed: reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@${REOLINK_UID}/main?transport=uid&local=192.0.2.10&broadcast=198.51.100.255
```

Use these URL forms:

```text
reolink://username:password@host[:port]/profile?[options]
reolink://username:password@camera-uid/profile?transport=uid[&options]
```

For LAN-P2P, put the camera UID in place of the host and add `transport=uid`. The adapter discovers that camera on the local network, then carries the Baichuan stream over reliable UDP. Escape URL-reserved characters in credentials.

Defaults are TCP port `9000`, channel `0`, the `main` profile, and enabled video, audio, and backchannel.

| Option | Values | Default | Purpose |
|---|---|---|---|
| TCP `:port` | `1`-`65535` | `9000` | Baichuan TCP port; invalid with UID transport |
| `profile` path | `main`, `sub`, `extern` | `main` | Camera stream; `extern` is a camera-dependent third profile |
| `stream` | `main`, `sub`, `extern` | `main` | Query-form alternative to the profile path; do not set both |
| `channel` | `0`-`255` | `0` | NVR or multi-lens channel |
| `transport` | `tcp`, `uid` | `tcp` | Direct TCP or local UID discovery and LAN-P2P over reliable UDP |
| `local` | IPv4 address assigned to this host | all active interfaces | Bind UID discovery and the resulting LAN-P2P socket to that address; UID only |
| `broadcast` | IPv4 address | each active interface's subnet broadcast | Override the UID discovery destination; UID only |
| `video` | `0`/`1`, `false`/`true` | enabled | Supply camera video to consumers |
| `audio` | `0`/`1`, `false`/`true` | enabled | Supply camera microphone audio to consumers |
| `backchannel` | `0`/`1` | enabled | Enable talkback to the camera speaker when supported |
| `options` | comma-separated list | per option | See [Stream options](#stream-options) |

With neither selector, UID discovery sends to each active IPv4 interface's subnet broadcast. `local` binds discovery and LAN-P2P traffic to one interface address; without `broadcast`, discovery uses that interface's subnet broadcast. `broadcast` overrides the destination. Both options are valid only with `transport=uid`. Each option may appear once; unknown options are rejected.
The profile names `mainStream`, `subStream`, `ext`, and `externStream` are also accepted for compatibility; the shorter names above are preferred.

`video=0` and `audio=0` suppress output tracks. The source still opens and drains the selected camera preview. At least one of video, audio, or backchannel must remain enabled. A talkback-only source requires camera talkback support.

### Stream options

Separate entries in `options` with commas. An option name by itself turns it on. Use `name:true` or `name:false` to set it explicitly.

| Option | Default | Applies to | What it does |
|---|---|---|---|
| `quickack` | Off | Linux TCP | May reduce stalls from delayed TCP acknowledgements, at the cost of one extra socket call per successful read and potentially more ACK traffic. |
| `sharedtcp` | Off | TCP | Shares a socket among profiles with the same group number, camera address, port, login, and transport settings. |
| `refetch` | On for eligible streams | TCP video with AAC audio or `audio=0` | After a drop, fetches buffered footage and resumes if it can verify where it fits. This checks media hashes and can take up to eight seconds per attempt. |

For example, `?audio=1&options=quickack,refetch:false` turns on QUICKACK and turns off refetch.

### Audio and talkback

`audio` and `backchannel` control opposite directions. They are not redundant.

- `audio=1` receives sound from the camera microphone.
- `backchannel=1` sends sound to the camera speaker.

If the connection stays healthy, a failed talkback capability probe does not block video or camera audio. Talkback is retried when the source reconnects. Talkback-only sources still require a successful probe.

| `audio` | `backchannel` | Result |
|---|---|---|
| `1` | `1` | Receive camera audio and send talkback |
| `1` | `0` | Receive camera audio only |
| `0` | `1` | Send talkback only |
| `0` | `0` | No audio in either direction |

For each camera channel, enable talkback on one profile. Use `main` for full-resolution live view or `sub` for a lower-bandwidth view. Set `backchannel=0` on the other profiles. Incoming audio can stay enabled on both, such as when another consumer uses the substream for sound detection.

Frigate switches to WebRTC for talkback. If your browser cannot play the H.265 main stream in WebRTC, set `backchannel=0` on `main` and `backchannel=1` on an H.264 profile such as `sub`. Open that view for talkback. You can keep recording the main stream. [Frigate live-view documentation](https://docs.frigate.video/configuration/live/)

Use this layout when you expose clear and fluent as separate streams:

```yaml
streams:
  camera_clear: reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@camera.local/main?audio=1&backchannel=1
  camera_fluent: reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@camera.local/sub?audio=1&backchannel=0
```

Use `camera_clear` for full-resolution video, listening, and talkback. The fluent stream still supplies incoming audio. Set `audio=0` on profiles where no consumer needs it. Clear and fluent target the same camera speaker when they use the same `channel` value.

### Connection layout

Set `options=sharedtcp:0` on the sources you want to share one TCP socket. Group numbers range from 0 to 255 and are local to each camera address, port, and login. Bare `sharedtcp` and `sharedtcp:true` select group 0. Omit the option or use `sharedtcp:false` for a dedicated connection.

For a dual-lens camera, assign both wide profiles to group 0 and both tele profiles to group 1. Each lens keeps its own socket for recording and detection:

| Profile | Camera channel | URL option |
| --- | --- | --- |
| Wide main and sub | `channel=0` | `options=sharedtcp:0` |
| Tele main and sub | `channel=1` | `options=sharedtcp:1` |

To group by profile instead, assign both main streams to group 0 and both substreams to group 1. Sharing is experimental. A socket failure can interrupt all profiles in its group. History requests, duplicate profiles, and profiles reopening while siblings still use the old connection use separate sockets.

A Reolink source uses one authenticated Baichuan connection. Its video, camera audio, and talkback share that connection. This is the recommended layout when one profile provides every required track:

```yaml
streams:
  camera: reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@camera.local/main?audio=1&backchannel=1
```

Use a second source if you need media from another profile. The following stream gets video from `main` and camera audio and talkback from `sub`. The default settings use separate connections.

```yaml
streams:
  camera:
    - reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@camera.local/main?audio=0&backchannel=0
    - reolink://${REOLINK_USER}:${REOLINK_PASSWORD}@camera.local/sub?video=0&audio=1&backchannel=1
```

The second source is a dedicated audio source. `video=0` prevents it from supplying video. It still receives camera audio because `audio=1`, and it supplies talkback because `backchannel=1`.

Set `audio=0` on the second source when you need a talkback-only source. The source still opens and drains its selected preview, so use `sub` to reduce bandwidth.

When several sources advertise talkback, go2rtc selects the first compatible source in listed order. Do not rely on that order. Enable `backchannel` on one source only. Separate named streams can each advertise talkback, but concurrent talk sessions on the same camera channel compete for one camera talk endpoint.

## Behavior

- One source serves all go2rtc consumers. Duplicate configured sources open separate camera sessions.
- Multiple configured sources retain go2rtc's ordered codec matching. A later H.264 source can serve a consumer that does not negotiate HEVC; `video=0` prevents a source from satisfying video.
- Connections are demand-driven. When the last consumer detaches, go2rtc stops the source and the adapter releases its preview, talk and receivers. A shared connection stays open while other previews use it.
- The adapter does not guess a camera's session capacity. The camera remains authoritative and may reject a new source when its model-, firmware-, or client-dependent limit is reached.
- A Baichuan login rejected with status 407 pauses retries for that camera and credential, starting at 15 seconds and doubling up to five minutes. A successful login clears the backoff.
- Failed TCP connections retry at most once per second per endpoint for the first three minutes, then back off up to five minutes. A successful TCP connection clears the backoff.
- Enabled audio must start within 10 seconds. A profile reconnects after 15 seconds without a valid audio frame. It also reconnects after a 60-second window below 75% of the advertised audio clock. Set `audio=0` when the camera microphone is absent or disabled, or incoming audio is unwanted.
- If video remains more than one second ahead of AAC across two distinct keyframes, the source reconnects. Valid video continues until the reconnect.
- Media framing errors trigger bounded recovery. Discarded or truncated media suppresses video until a valid keyframe. The source reconnects if recovery fails.
- H.264 setup includes the camera's decoder parameters even if the startup keyframe lists them out of order. When the keyframe identifies a matching pair, go2rtc advertises that pair first. If an inter frame carries unfamiliar parameters, go2rtc waits for a compatible keyframe before forwarding video again.
- During media recovery, video profiles configured with `audio=0` can recover timestamp gaps up to 60 seconds after two validated keyframes with matching codec settings. Audio-enabled profiles keep the 30-second clock limit. This recovery does not synchronize audio from a separate source.
- Best-effort setup queries run concurrently per configured source and add compact model, firmware, channel, and discovery status. Rejected queries and query timeouts do not prevent streaming. Malformed or oversized replies can close the connection.
- Repeated talkback negotiation, writes, and teardown are tested. Audible quality, echo, and client UI behavior remain experimental.

## Diagnostics

The `reolink` producer object shows the recovery counters. They start at zero for each producer generation.

- `boundary_adjustments` counts I-frame boundaries that differ from declared sizes. This includes padding repairs.
- `short_video_units` counts shortened I-frame units detected by the parser, including during startup.
- `video_resyncs` counts successful returns to synchronized video.
- `refetch_attempts` counts tries to fetch buffered camera footage. `refetch_resumes` counts attempts accepted for replay after go2rtc verifies where the footage fits. `refetch_enabled` shows when the source can use this recovery path.

A shortened frame can count in both `boundary_adjustments` and `short_video_units`. For troubleshooting, include the `reolink` object, the camera model and firmware, and the affected profile. For VAAPI failures, include the GPU and driver versions and redacted Frigate and go2rtc logs around the first error.

## Unsupported (so far)

- WAN or cloud P2P, relay, NAT traversal, and UPnP
- Battery-camera wake and cloud lifecycles
- PTZ, presets, lights, sirens, camera settings, and event APIs; existing ONVIF or camera integrations remain separate
- Automatic RTSP, RTMP, HTTP-FLV, or FFmpeg fallback

## Security and limits

- Use only on a trusted network. Baichuan does not authenticate the camera before login negotiation. It may select an unencrypted session.
- A hostile broadcast-domain peer can impersonate a camera during UID discovery and attempt offline password guessing.
- UID/LAN-P2P defaults to one IPv4 broadcast domain. Routed discovery requires directed-broadcast and return routes.
- Keep credentials in environment substitutions.
- Media frames are limited to 16 MiB. Recovery searches bounded windows; these limits have no URL options.

## Thanks

- Thanks to quenthal for suggesting that talkback use the preview connection. [PR discussion](https://github.com/AlexxIT/go2rtc/pull/2413#issuecomment-5281069771)
- Thanks to etvanp for reporting and retesting the fix for larger Baichuan login replies. [PR discussion](https://github.com/AlexxIT/go2rtc/pull/2413#issuecomment-6166552776)
- Thanks to mhoogenbosch for the reports and patches that informed talkback session isolation, ADPCM block sizing, browser audio resampling, and muted-microphone handling. [PR discussion](https://github.com/AlexxIT/go2rtc/pull/2413#issuecomment-6727321058)
- Thanks to gjas24 for testing camera support and sharing the H.264 workaround for WebRTC talkback. [PR discussion](https://github.com/AlexxIT/go2rtc/pull/2413#issuecomment-6862976195)
- Thanks to benbraun for the reports and patches that informed login cooldown, defensive session cleanup, username masking and optional TCP sharing. [PR discussion](https://github.com/AlexxIT/go2rtc/pull/2413#issuecomment-7046803787)
- Thanks to MaximVB86 for testing the RLC-823S2 and Video Doorbell PoE and reporting the Intel VAAPI issue. [PR discussion](https://github.com/AlexxIT/go2rtc/pull/2413#issuecomment-7095339748)
