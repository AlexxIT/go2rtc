# Local Changes

This fork's changes focus on **improving the reliability of the
Nest → go2rtc → UniFi Protect pipeline**. Out of the box, that
end-to-end path has several latent problems: UniFi adoption doesn't
negotiate audio, RTSP keepalive methods aren't acknowledged so
sessions die every ~30 seconds, the Nest stream extension is
fire-once so the stream silently dies at ~10 minutes, and when the
upstream Nest source reconnects, downstream UniFi consumers get stuck
on stale codec parameters and freeze until manual restart.

The patches in this file address each of those layers — ONVIF profile
contents, RTSP method handling (RFC 2326 compliance), Nest stream
extension lifecycle, WebRTC keyframe requests, and consumer-refresh
on source reconnect — along with observability improvements
(structured logs with remote IP / User-Agent, info/warn levels for
source-outage events, credential redaction of source URLs in log
output).

The fixes are general-purpose: they help any RFC-2326 RTSP client
talking to go2rtc (UniFi, Frigate, Synology Surveillance, Axis, etc.)
and any go2rtc Nest source, not just the Nest+UniFi combination that
exposed them.

Each item below is independently revertable. Tested against
UniFi Protect 7.1.60 + [Google Nest Battery Camera](https://store.google.com/gb/product/nest_cam_battery?hl=en-GB)
as the primary client/source combination.

---

## ONVIF

### 1. ONVIF profile advertises AAC audio

- **Files:** `pkg/onvif/server.go`
- **Change:** `appendProfile` now emits `<tt:AudioSourceConfiguration>` and
  `<tt:AudioEncoderConfiguration>` (AAC, 48 kHz, 64 kbps) alongside the
  existing video configurations. New top-level builders
  `GetAudioSourcesResponse`, `GetAudioSourceConfigurationsResponse`,
  `GetAudioEncoderConfigurationsResponse` replace the previous empty stubs.
- **Why:** ONVIF clients that gate audio track setup on profile contents
  (UniFi Protect 3rd-party adoption is one) wouldn't issue RTSP SETUP for
  the audio track if no `AudioEncoderConfiguration` was present, even
  though the SDP from DESCRIBE included an audio track.
- **Impact:** UniFi Protect (and similarly strict ONVIF clients) now show
  the "Enable Audio" toggle and successfully negotiate the audio stream.

### 2. Singular AudioSourceConfiguration / AudioEncoderConfiguration

- **Files:** `pkg/onvif/server.go`
- **Change:** Added `GetAudioSourceConfigurationResponse` and
  `GetAudioEncoderConfigurationResponse` (singular variants). Added
  matching operation constants and `StaticResponse` routing.
- **Why:** ONVIF Media Service spec requires both singular and plural
  variants of every Configuration getter. Previously only the plural
  forms were implemented; clients probing the singular forms got 400.

### 3. Proper POSIX TZ format

- **Files:** `pkg/onvif/helpers.go`
- **Change:** Rewrote `GetPosixTZ` to produce IEEE 1003.1 POSIX TZ
  strings (`UTC0`, `EST5`, `JST-9`) instead of the previous non-standard
  `GMT±HH:MM` format. Sign convention now follows POSIX (reversed from
  ISO 8601). Zone name comes from Go's `time.Zone()` and respects the
  container's `$TZ` env var or `/etc/timezone`.
- **Why:** The previous output was non-conformant per ONVIF spec which
  references POSIX. Strict clients ignored it; clients that parsed it
  saw incorrect signs for east-of-UTC zones.

### 4. Imaging service stub

- **Files:** `pkg/onvif/server.go`, `pkg/onvif/envelope.go`,
  `internal/onvif/onvif.go`
- **Change:** Added imaging service to `GetCapabilities` and
  `GetServices` advertisements. Added stub responses for
  `GetImagingSettings`, `GetOptions`, `GetMoveOptions`, `GetStatus`, and
  `GetServiceCapabilities` (the latter routed by URL path since it's
  shared between Media and Imaging services). Added `timg:` namespace
  to the SOAP envelope prefix.
- **Why:** Some clients (Frigate, newer Synology Surveillance) probe the
  imaging service during adoption. Without it they logged warnings and
  occasionally failed adoption. Stub responses (empty configurations)
  cleanly declare "no imaging features supported".

### 4b. Documented: ONVIF server does not authenticate requests

- **Files:** `internal/onvif/README.md`
- **Change:** Added a "Security" subsection to the ONVIF README
  documenting that go2rtc's ONVIF server dispatches every
  operation without parsing or verifying the SOAP
  `wsse:UsernameToken` element. Credentials prompted for by
  NVR/HA adoption tools (UniFi Protect, Home Assistant, etc.) are
  decorative; any value succeeds. The note covers what IS still
  protected (the underlying RTSP/HTTP stream URLs, which carry
  their own auth) and what is NOT (camera-setup metadata
  enumeration via `GetProfiles`,
  `GetVideoEncoderConfiguration`, etc.). Recommends treating the
  go2rtc API port as trusted-network only.
- **Why:** Operators integrating go2rtc with NVRs typically see
  a credential prompt during adoption and assume those
  credentials are validated server-side. They aren't — and
  exposing the ONVIF endpoint beyond a trusted network would
  leak camera-setup details to unauthenticated callers. This is
  upstream behaviour, not specific to this fork; the note exists
  to make the implicit explicit. No code change.
- **Superseded by [4c](#4c-ws-security-usernametoken-authentication-for-onvif-server):**
  this fork has since implemented WS-Security `UsernameToken`
  validation (see below) rather than waiting on upstream PR
  https://github.com/AlexxIT/go2rtc/pull/2231.

### 4c. WS-Security UsernameToken authentication for ONVIF server

- **Files:** `pkg/onvif/envelope.go`, `pkg/onvif/server.go`,
  `internal/onvif/onvif.go`, `internal/onvif/README.md`
- **Change:** New `onvif.username` / `onvif.password` config keys
  (separate from the WebUI/JSON-API `api.username` /
  `api.password` Basic auth). When set, `VerifyUsernameToken`
  checks every ONVIF operation's `wsse:UsernameToken` header
  (PasswordDigest: `Base64(SHA1(nonce + created + password))`)
  against the configured credentials, rejecting stale
  (>5 minute skew) or mismatched tokens with a SOAP
  `ter:NotAuthorized` fault and HTTP 401.
  `GetSystemDateAndTime` is exempted so clients can learn the
  server's clock before computing a digest against it. Digest
  comparison uses `crypto/subtle.ConstantTimeCompare` (not `==`)
  to avoid a timing side-channel, and a bounded in-memory cache
  of seen (username, nonce, created) triples rejects replays of
  a captured token within its freshness window; a future-dated
  `Created` gets only 30s of clock-skew grace rather than the
  full 5-minute window.
- **Why:** The ONVIF server previously dispatched every operation
  regardless of credentials (see 4b). Real ONVIF clients (NVRs,
  Home Assistant) authenticate via WS-Security, not HTTP Basic,
  so `api.username`/`api.password` can't protect this endpoint —
  dedicated ONVIF-native auth was needed.

---

## RTSP Server

### 5. GET_PARAMETER / SET_PARAMETER during session setup

- **Files:** `pkg/rtsp/server.go`, `pkg/rtsp/conn.go`
- **Change:** Added `MethodGetParameter` / `MethodSetParameter` constants
  and a case in the `Accept()` method switch that returns `200 OK` for
  both. Added these methods to the `Public:` header in the OPTIONS
  response.
- **Why:** RFC 2326 §10.8/§10.9 define these as keepalive methods.
  Before this fix the server fell through to `default` which returned
  an error and closed the TCP connection, killing any client (UniFi
  Protect, Axis cameras) that uses these as keepalives during setup.

### 6. GET_PARAMETER / SET_PARAMETER during steady state (after PLAY)

- **Files:** `pkg/rtsp/conn.go`
- **Change:** The `handleTCPData` steady-state read loop recognised
  `GET_`/`SET_` prefixes but only emitted responses for `OPTIONS`.
  Extended to respond `200 OK` to `GET_PARAMETER`, `SET_PARAMETER`, and
  `PAUSE` in steady state. Added `TEARDOWN` handler that acknowledges
  and closes the connection. Unknown methods now get
  `455 Method Not Valid In This State`.
- **Why:** This was the most impactful fix of the session. UniFi Protect
  sends `SET_PARAMETER` every ~30s as a keepalive. Previously the request
  was read and logged via the listener but no response was written, so
  UniFi's request timeout expired, UniFi tore down the TCP connection,
  and the consumer cycled into reconnect → mid-GOP green frames →
  "Camera Lost Wired Connection" indefinitely. This fix made UniFi
  sessions hold indefinitely.

### 7. RFC 2326 compliance pass

- **Files:** `pkg/rtsp/server.go`, `pkg/rtsp/conn.go`
- **Changes:**
  - Unknown methods now return `501 Not Implemented` with an `Allow:`
    header (was: connection-drop). Per RFC 2326 §11.4 and §1.4.
  - `PAUSE` returns `200 OK` rather than falling through to the unknown
    method case.
  - Session ID is now generated only on the **first** SETUP within a
    client session; subsequent SETUPs reuse the same ID. Per RFC 2326
    §10.4. Previously each SETUP generated a fresh random ID.

---

## WebRTC Source

### 8. PLI requests for ActiveProducer + immediate PLI on track setup

- **Files:** `pkg/webrtc/conn.go`
- **Change:** The existing periodic-PLI ticker (which fires every 2s for
  PassiveProducer video tracks) now also fires for `ModeActiveProducer`
  (e.g. Nest, where go2rtc dials out to a cloud SFU), at 10s intervals
  to balance keyframe freshness against upstream bandwidth/airtime. Also
  sends an immediate PLI when the track is first set up, so cold-start
  recovery doesn't have to wait for the first ticker fire.
- **Why:** WebRTC SDP doesn't usually include `sprop-parameter-sets`, so
  downstream RTSP consumers can't decode anything until an IDR (carrying
  inline SPS/PPS) arrives. Without PLI requests, IDR cadence is on the
  upstream sender's schedule — typically 30–60s for Google's Nest SFU.
  This caused extended pixelation after a go2rtc restart while UniFi
  reconnected and waited for Google's next scheduled keyframe. With the
  fix, cold-start recovery is typically <5s.

### 8b. Kick downstream consumers on Producer reconnect

- **Files:** `internal/streams/producer.go`, `internal/streams/stream.go`,
  `internal/streams/play.go`, `internal/streams/stream_test.go`,
  `pkg/core/connection.go`
- **Change:** Added a `stream *Stream` back-reference to `Producer` so
  it can notify its parent stream after a successful reconnect.
  Stream gains a `kickConsumers(reason string)` method that snapshots
  the consumer list, gathers the remote addresses via a small
  `remoteAddrer` interface (satisfied by anything embedding
  `*core.Connection`), and calls `Stop()` on each consumer. The
  reconnect path in `Producer.reconnect()` invokes this (in a
  goroutine to avoid holding the producer mutex during downstream
  teardown). `core.Connection` gains a `GetRemoteAddr()` accessor
  method that makes the address available via the interface without
  introducing an import cycle.
- **Why:** When a Producer (e.g. Nest WebRTC) reconnects, the new
  source typically has different codec parameters (SPS/PPS in a fresh
  WebRTC SDP). The existing `receiver.Replace(track)` swaps the
  upstream track silently — downstream RTSP consumers (UniFi Protect)
  keep their session alive but their decoder is configured for the
  *previous* SDP. New RTP frames arrive with incompatible bitstream
  parameters → frozen video until manual reconnect.
  By kicking consumers on reconnect, they're forced to re-DESCRIBE
  and pick up the new producer's SDP. The natural list cleanup happens
  in each consumer's transport handler (e.g. `tcpHandler` in
  `internal/rtsp` calls `RemoveConsumer` when its loop exits).
- **Grace period:** When the last consumer disconnects (which happens
  cascade-style during a kick), `stopProducers()` would normally tear
  down the producer we just reconnected — and a slow reconnect from
  the kicked client would then have to wait for a fresh cold-start of
  the producer (~6s for Nest WebRTC), often timing out and entering
  a long back-off cycle (observed 7-minute recording gaps).
  To prevent this, `kickConsumers()` bumps the `s.pending` counter
  for a 2-minute grace window. `stopProducers()` already
  short-circuits when `pending > 0`, so the producer stays warm and
  the reconnecting client lands cleanly on it.
  2 minutes is sized for the worst case observed in practice: when
  UniFi Protect's RTSP session closes shortly after opening (e.g. a
  natural reconnect followed by a kick ~10s later), it interprets
  this as server instability and enters an extended retry back-off
  (empirically ~60–90s before re-DESCRIBE). An earlier 30s grace
  window expired during this back-off, the producer stopped, and
  when UniFi finally retried it hit a cold producer and timed out
  again — repeating the recording gap. 2 minutes covers this case
  with margin. Trade-off: an orphan producer keeps streaming from
  upstream for up to 2 minutes (~50–200 kbps for a typical Nest
  stream); cheap compared to recording gaps that require manual
  intervention.
- **Internal-loop consumer skip:** Recursive pipelines like
  `ffmpeg:driveway` (an ffmpeg subprocess that reads from
  `rtsp://localhost/driveway` and is registered as a consumer of the
  driveway stream while *also* being a producer of it) need special
  handling. Kicking the inner ffmpeg subprocess on producer reconnect
  killed its stdout → producer EOF'd → triggered another reconnect →
  kicked again → infinite ~5-per-second loop. Fix: in
  `kickConsumers()` skip any consumer whose `GetSource()` matches one
  of this stream's producer URLs. Matches the existing loop-protection
  check in `AddConsumer`.
- **Earlier failed attempt (now reverted):** A `SetReadDeadline()`-
  based fast silence detection (commits 58f1495, 9a1ad59, 1254ea9)
  tried to reduce the ~95s detection latency for terminal silences.
  The 25s fast recovery paradoxically broke UniFi because it bypassed
  the natural session-timeout recovery path that the 95s detection
  relied on. The kick-consumers fix in this commit addresses the
  underlying codec-refresh problem directly, so fast detection could
  be safely re-introduced in a follow-up if desired.
- **Test coverage:** `TestKickConsumers` (6 sub-tests: empty,
  multiple, no list mutation, internal-loop consumer is skipped,
  no grace-period bump when only internal consumers,
  grace-period pending bump/release),
  `TestNewStreamLinksProducers` (3
  sub-tests: single string, []string, []any construction paths),
  `TestRedactSourceURL` (6 sub-tests covering nest, rtsp with auth,
  ffmpeg, fragment-only, empty). Uses a `mockConsumer` implementing
  `core.Consumer` with an atomic Stop counter.

### 8b-1. Kick on reconnect: disabled by default (UniFi 7.1.69 regression)

- **Files:** `internal/streams/producer.go`, `internal/streams/streams.go`
- **Change:** The auto-kick described in section 8b is now gated by
  the `GO2RTC_KICK_ON_RECONNECT` environment variable. Default:
  **off**. At startup the active mode is logged at info level
  (`[streams] kick on producer reconnect: disabled — set
  GO2RTC_KICK_ON_RECONNECT=true to re-enable for legacy RTSP
  clients`).
- **Why:** Empirical regression after upgrading UniFi Protect to
  7.1.69. A nest WebRTC reconnect at 19:01:36 triggered the kick
  (correct: 2 UniFi RTSP sessions closed). Pre-7.1.69, UniFi
  re-DESCRIBEd within 1–3s. On 7.1.69, **UniFi made zero RTSP
  reconnect attempts** during the entire 2-minute grace window
  even though the snapshot poll endpoint kept responding — so the
  recording session was severed permanently and the grace period
  bandaged no wound. The new heartbeat (section 13) confirmed
  nest's H264/OPUS packet rates were fully healthy after reconnect;
  the only thing missing was UniFi.
- **Hypothesis:** UniFi 7.1.69 either (a) interprets a
  server-initiated TCP close as intentional and stops retrying,
  or (b) trusts its GStreamer 1.26+ H.264 parser to handle
  in-band SPS/PPS changes via RTP. Either way, the kick is now
  counterproductive for that client. Pre-7.1.69 UniFi and other
  RTSP clients that genuinely can't handle in-band parameter
  changes can opt back in with the env var.
- **Trade-off:** If the new nest connection's SPS/PPS differ from
  the previous one and the RTSP client cannot pick up the change
  from in-band parameter sets, video stays frozen until the
  client itself disconnects (UniFi 7.1.69 reportedly does this
  on its own when frames stop arriving — needs verification in
  next outage).
- **No test changes:** The existing `TestKickConsumers` exercises
  `kickConsumers()` directly and is unaffected by the gate, which
  sits one layer up in `Producer.reconnect()`.

### 8c. Streams logging: info/warn levels + credential redaction

- **Files:** `internal/streams/producer.go`, `internal/streams/stream.go`
- **Change (logging levels):** Source reconnect cycles are now
  surfaced at multiple log levels for operational visibility:
  - `INF [streams] producer reconnecting source=<scheme>:` fires once
    at the start of a reconnect cycle (retry=0). Lets operators
    running at `log.level=info` see source-outage events without
    enabling debug.
  - `WRN [streams] producer reconnect still failing source=… retry=5`
    fires once when retries pass the 5-second-backoff threshold, so
    persistent outages bubble up to warn-level without per-retry
    noise.
  - `INF [streams] producer reconnected source=…` fires after a
    successful reconnect, pairing with the "reconnecting" line to
    confirm recovery happened.
  - Per-retry `[streams] retry=N to url=…` lines stay at debug to
    avoid log volume when a source is flapping.
- **Change (kick log):** The `[streams] kicking consumers` log line
  now includes a `remotes=` array so a single grep on the kick event
  tells you exactly which downstream clients were notified, no
  cross-referencing with subsequent `[rtsp] disconnect` lines needed.
- **Change (credential redaction):** Added a `redactSourceURL()`
  helper that strips the URL query (and fragment) before logging.
  Applied to all `p.url` occurrences in `internal/streams/producer.go`
  (5 existing log calls plus the 3 new info/warn ones). Source URLs
  like `nest:?client_id=…&client_secret=…&refresh_token=…` are now
  logged as `nest:` only. Other credential-bearing schemes
  (RTSP with `user:pass@host`) were already covered by
  `pkg/creds.SecretWriter`'s `userinfoRegexp`; this fix closes the
  query-string credential leak.
- **Why:** The pre-fix logs printed full producer URLs at multiple
  levels including warn. For Nest sources the URL carries OAuth
  credentials in the query string — sharing any log file (e.g.
  pasting into an issue tracker or chat) leaked the secret. With the
  redaction, log lines stay useful for diagnosis (you can still see
  the source scheme + path) but no credentials appear.
- **Deliberately not redacted:** `[echo]` log of script output, `[expr]`
  source URL, `[api] request` query strings, `[onvif]` SOAP bodies.
  Each is either lower-risk (PasswordDigest is an HMAC, not plain
  text) or has legitimate non-credential information that would be
  lost by blunt redaction. Worth revisiting individually if a specific
  use case requires it.

---

## Nest Source

### 9. Stream extension loops

- **Files:** `pkg/nest/api.go`
- **Change:** The `StartExtendStreamTimer` goroutine now loops, re-arming
  the timer after each successful `ExtendStream()`. Previously the
  goroutine waited for one timer fire, called `ExtendStream()` once, and
  exited — so the stream died at the second Google-side expiry (~10 min
  after connect).
- **Why:** Google Nest Device Access streams expire ~5 minutes after the
  last extension and must be re-extended before then. A single extension
  bought 5 more minutes but no further; the loop now keeps the stream
  alive indefinitely.

### 10. Race-free Stop + retry on extension failure

- **Files:** `pkg/nest/api.go`
- **Change:** Added an `extendStop` channel to the `API` struct.
  The extension goroutine uses `select` on both the timer and the stop
  channel so `StopExtendStreamTimer()` cleanly unblocks the goroutine.
  Timer reference is captured locally so a Stop-then-Start race can't
  produce a nil-pointer panic on `Reset()`. On `ExtendStream()` failure,
  the goroutine retries once after 10 seconds (interruptibly) before
  giving up. Added an `extendDelay` helper that clamps the next-wakeup
  duration to a 1-second minimum to prevent busy-looping if Google ever
  returns a stale expiry.
- **Why:** The previous implementation had a latent nil-deref race
  between `Stop()` and the goroutine's `Reset()` call, and no resilience
  against transient extension failures (a single Google 5xx would kill
  the stream until next reconnect).

### 10b. OAuth 401 refresh in ExtendStream + token-cache eviction fix

- **Files:** `pkg/nest/api.go`
- **Change:** Two coupled fixes around Google OAuth bearer-token expiry.
  - **`ExtendStream()` now handles 401.** On a 401 response, it calls
    `refreshToken()` and retries the request once with the new token.
    Mirrors the existing 401 handling in `ExchangeSDP()`. The retry
    uses a helper closure so the same `*http.Request` body isn't
    drained twice.
  - **`refreshToken()` rewritten to actually refresh.** The previous
    implementation called `NewAPI()` to "get a new token" — but
    `NewAPI()`'s cache returns the same `*API` instance when the
    recorded `ExpiresAt` is still in the future, so the "refreshed"
    token was identical to the stale one. The new implementation
    bypasses the cache entirely: POSTs directly to the Google OAuth
    token endpoint and mutates `a.Token` + `a.ExpiresAt` in place.
    Serialized via a new `refreshMu` so concurrent 401s fold into a
    single refresh request.
- **Why:** Google OAuth access tokens for the Smart Device Management
  API expire after ~60 minutes. Each Nest stream session also runs
  up to ~60 minutes, so it's normal for an extend to land on a
  freshly-expired token. Without the fix, an extend after the OAuth
  boundary 401s, the broken `refreshToken()` no-ops, the retry 401s
  again, the extend goroutine gives up, and the stream silently dies
  at its next `expires_at`. With the fix, the same boundary now
  shows `[nest] extend got 401, refreshing OAuth token` →
  `[nest] OAuth token refreshed` → `[nest] extend ok` in quick
  succession; stream continues uninterrupted.

### 10c. ExtendStream 409/429 transient-status retry with exponential backoff

- **Files:** `pkg/nest/api.go`, `pkg/nest/api_test.go`
- **Change:** `ExtendStream()` is now a retry loop (maxRetries=3)
  that distinguishes three retryable status classes:
  - **401 Unauthorized** — token expired; refresh and retry
    immediately (existing behaviour from §10b, now folded into the
    same loop).
  - **409 Conflict / 429 Too Many Requests** — transient server-side
    condition; back off and retry. Initial back-off is 30 s,
    doubled on each subsequent failure. Exposed as the package
    variable `extendBackoffInitial` so tests can shorten it.
  - **5xx and other non-retryable codes** — surface as error
    immediately (not retried; the producer-level reconnect
    machinery is the right place to absorb those).
  Each retry attempt logs at warn level with the status, attempt
  count, max attempts, and backoff duration.
- **Why:** Google's SDM API returns 429 under client-side rate
  limiting (which an aggressive extend loop can trigger after
  many proactive reconnects) and 409 under transient session
  conflicts. The previous implementation would surface either as
  a hard error, killing the extend goroutine and forcing the
  stream to die at its current `expires_at`. With the retry
  loop, short-lived API hiccups are absorbed without disrupting
  the stream.
- **Test coverage:** `TestExtendStreamRetry` (6 sub-tests) drives
  the retry loop with an `httptest` server hooked in via the new
  `extendURI` package var: happy path (200), 429-then-200,
  409-then-200, retry exhaustion (three consecutive 429s), 5xx
  fail-fast, and backoff doubling between attempts. Uses
  `extendBackoffInitial = 1ms` to keep tests fast.

### 10d. Nest API hardening: HTTP timeouts + PeerConnection leak fixes

- **Files:** `pkg/nest/api.go`, `pkg/nest/client.go`,
  `internal/nest/init.go`
- **Change:** Three latent reliability issues in the Nest API
  surface, fixed together:
  - **HTTP client timeout: `time.Second * 5000` → `10 * time.Second`**
    in every `*http.Client` constructed by `NewAPI`, `GetDevices`,
    `ExchangeSDP`, `GenerateRtspStream`, `StopRTSPStream`, and
    `ExtendStream`. The 5000-second value is almost certainly a
    typo for 5; with it, any hung Google API call could block a
    go2rtc goroutine for ~83 minutes before timing out.
  - **`PeerConnection` cleanup on `rtcConn` error paths.** The
    WebRTC dial routine now calls `pc.Close()` whenever
    `CreateCompleteOffer`, `ExchangeSDP`, or `SetAnswer` fails
    before returning. Without this, every failed dial attempt
    leaked a `PeerConnection` (UDP sockets, ICE state, DTLS
    context); over many retry cycles under upstream throttling
    or transient network issues this would slowly exhaust the
    process's UDP-port and goroutine budget. Each close logs
    at debug level with the original error for traceability.
  - **`defer res.Body.Close()` added to `GenerateRtspStream` and
    `StopRTSPStream`.** The original code only closed the response
    body via the implicit JSON-decoder path on the success branch;
    a non-200 response or a decode error left the body open,
    leaking the underlying connection back to the pool.
  - **Typo fixes:** `cliendID` → `clientID`, `cliendSecret` →
    `clientSecret`, `compataiility` → `compatibility` in
    `pkg/nest/client.go` and `internal/nest/init.go`.
- **Why:** These bugs are independent of the OAuth/extend logic
  we already fixed, but they all affect the long-term resource
  hygiene of the Nest integration. The 5000-s timeout is the
  most consequential — under any Google-side hang it would
  silently park a goroutine for over an hour, holding state and
  delaying error detection well past the points where the
  proactive-reconnect (§12b) or extend-retry (§10c) machinery
  could otherwise help.
- **Source:** Items independently identified in the upstream PRs
  https://github.com/AlexxIT/go2rtc/pull/2194 and
  https://github.com/AlexxIT/go2rtc/pull/2128.
- **Note (lessons learned):** These two upstream PRs were not
  discovered until *after* the bulk of the Nest/UniFi reliability
  work in this fork (§8b through §15) had been written and
  deployed. Both PRs predate this fork's troubleshooting and
  would have shortcut several rounds of diagnosis — particularly
  the 5000-second HTTP timeout, which we missed entirely until
  reading PR #2194. Future investigations into the Nest
  integration (or any other upstream-maintained source) should
  start with a scan of the upstream issue tracker and open PRs
  before reaching for the debugger; this would have saved
  multiple debugging sessions here.

---

## Observability

### 11. RTSP debug logs: source IP + User-Agent

- **Files:** `internal/rtsp/rtsp.go`
- **Change:** Added `remote=` (peer address) and `user_agent=` (from the
  first request on the connection) structured fields to:
  - `[rtsp] new consumer`
  - `[rtsp] new producer`
  - `[rtsp] handle <error>`
  - `[rtsp] disconnect`
  - `[rtsp]` consumer add-track error
- **Why:** When multiple clients connect to a single stream (UniFi from
  several IPs, an internal ffmpeg loop, an occasional VLC test), the
  default `[rtsp] new consumer stream=driveway` log line gave no way to
  tell which session was which. With the new fields, sessions are
  trivially greppable by `remote=192.168.5.1` or
  `user_agent=GStreamer/1.26.10`.

### 11b. Warn on RTSP DESCRIBE / ANNOUNCE of unknown stream

- **Files:** `internal/rtsp/rtsp.go`
- **Change:** When an RTSP client requests a stream name that doesn't
  exist in `streams.Get(name)`, the handler now logs a warn-level line
  with stream name, remote address, and user-agent before returning.
  Previously the request was rejected silently.
- **Why:** Most common cause is a configuration mismatch — the client
  (UniFi Protect, VLC, Frigate) is pointed at a stream name that
  doesn't exist in the current go2rtc.yaml. Symptom from the client
  side is "can't adopt" or "stream unavailable" with no go2rtc-side
  signal to confirm. The warn line ("stream not found",
  "stream not found (ANNOUNCE)") gives operators an immediate
  pointer at the problem without enabling debug.

### 12. Structured logging across HTTP / WebSocket / ONVIF / MP4

- **Files:** `internal/onvif/onvif.go`, `internal/api/api.go`,
  `internal/mp4/mp4.go`, `internal/api/ws/ws.go`
- **Change:** Converted positional log lines to structured fields with
  `remote=`, `user_agent=`, and operation-specific context:
  - `[onvif] server request` (trace) — adds `remote`, `user_agent`, `op`
  - `[onvif] unsupported operation` (warn) — adds `remote`, `user_agent`, `op`
  - `[api] request` middleware (trace) — restructured with `method`,
    `url`, `remote`, `user_agent`
  - `[mp4] request` handler (trace) — restructured with `method`, `url`,
    `remote`, `user_agent`
  - `[api] ws upgrade` (error) — adds `remote`, `user_agent`
  - `[api] ws msg` (trace) — adds `remote`
- **Why:** Consistent observability across all client-facing surfaces.
  Same grep patterns work for any client interaction regardless of
  protocol.

### 12b. Proactive reconnect ahead of Nest's stream-lifetime cap

- **Files:** `pkg/core/core.go`, `internal/streams/producer.go`,
  `pkg/nest/client.go`
- **Change:** Two-part addition that eliminates the ~30-40 s
  recording gap that recurred every ~66 minutes on the Nest
  → go2rtc → UniFi Protect pipeline:
  - **`core.LifetimeLimited` interface** — an optional hook
    `SetReconnectCallback(cb func())` that producers implement to
    receive a proactive-reconnect trigger from the wrapping
    `streams.Producer`. Generic, not Nest-specific; any source
    with a known stream-lifetime cap can opt in.
  - **`streams.Producer.proactiveReconnect()`** — bumps `workerID`
    so the soon-to-exit old worker's post-Start `reconnect()`
    call no-ops, then runs the standard `reconnect()` flow. That
    flow is already overlap-friendly: new conn via `GetProducer`,
    tracks moved via `Receiver.Replace()` before the old conn is
    stopped. Consumers see only the brief `GetProducer` window
    (~3 s for Nest), not the previous ~30-40 s detection chain.
    Callback registration happens in `start()` (initial conn)
    and again in `reconnect()` (post-swap onto the new conn) so
    every lifetime cycle is caught.
  - **Nest `WebRTCClient` / `RTSPClient` implement
    `LifetimeLimited`.** Each `Start()` arms a one-shot
    `time.AfterFunc(refreshBeforeCap, ...)` (default: 55 minutes,
    package-var for tests). When the timer fires, the wrapped
    producer's `proactiveReconnect()` is invoked in a fresh
    goroutine so the timer runner isn't blocked by the network
    work. `Stop()` cancels the timer. `sync.Once`-like
    `refreshFired` guard prevents double-fire under odd shutdown
    orderings.
- **Why:** Google's Nest Smart Device Management API caps a single
  WebRTC stream at ~60 minutes from initial `GenerateWebRtcStream`,
  regardless of how often `ExtendWebRtcStream` is called. Past
  that cap, the upstream silently stops delivering packets and the
  current reconnect path takes ~30-40 s to notice and recover
  (inner-ffmpeg 5 s read timeout → 3 retries × 10 s → finally
  declare EOF → producer reconnect). Without proactive refresh,
  recording gaps recur on a roughly ~66-minute cadence (the 60-min
  cap plus a few minutes of residual delivery and detection
  latency). A proactive refresh at 55 min hits while upstream is
  still healthy, allowing the swap to complete inside the
  producer's already-overlap-friendly reconnect flow before the
  old session goes silent.
- **Why a generic interface, not a Nest-only hook:** The same
  pattern applies to any cloud-mediated stream with a session
  lifetime (Ring, Arlo, Tuya, etc. all have analogous limits).
  Putting `LifetimeLimited` on `pkg/core` lets each producer opt
  in without touching the streams package — only the producer
  itself knows its lifetime contract.
- **Safety:** `workerID` bump under the producer mutex guarantees
  the old worker's post-Start `reconnect()` no-ops; otherwise it
  would race the proactive reconnect into producing a second
  fresh connection. The previous overlap-friendly reconnect flow
  is unchanged.
- **No new tests:** The wiring is small (callback registration on
  start/reconnect, a one-shot timer in nest client). The
  user-facing test is the next ~66 min interval after
  deployment: the gap should either disappear or shrink
  to seconds.

### 13. Producer activity heartbeat + stopProducers decision trace

- **Files:** `internal/streams/producer.go`, `internal/streams/stream.go`
- **Change:** Two diagnostic logging additions targeting the
  "silent stall" failure class — where upstream looks healthy
  (no EOF, no read-deadline fire) and consumers stay attached, but
  recording freezes anyway.
  - **Producer activity heartbeat (debug, every 60s).** Each active
    producer logs one line per receiver with packet/byte deltas since
    the previous tick: `[streams] producer activity source=…
    track=0 codec=H264 dpackets=N dbytes=N total_packets=N`. When
    `dpackets=0` for a track while a sibling track keeps ticking, the
    smoking gun is unambiguous (e.g. video frozen, audio flowing —
    a UniFi+Nest failure mode where prior logs were silent).
    Heartbeat runs in its own goroutine started from `start()`,
    exits when `workerID` advances (next cycle or stop). Cadence
    exposed as `activityInterval` for tests.
  - **stopProducers decision trace (trace).** Each call now logs
    `[streams] stopProducers stopped=N kept=N` (skipping the line
    only when both are zero), so a `RemoveConsumer` that kept all
    producers alive because senders held them up is distinguishable
    from one that wasn't triggered. Previously the function was
    silent in that case.
  - **Consumer-removed remaining count (trace).** `RemoveConsumer`
    emits `[streams] consumer removed remaining=N`. The per-transport
    disconnect log (`[rtsp] disconnect …`) records who left but not
    the stream's resulting consumer count — without that, a
    `producer.stop()` that follows can't be tied to the last-consumer
    teardown vs. one of several simultaneous removals.
- **Why:** Two blind spots in the existing logs made the "silent
  stall" class of failures hard to diagnose:
  1. After a kick + reconnect, ffmpeg ADTS warnings (audio) kept
     appearing for many minutes — so audio was flowing — but there
     was no way to tell whether video was also flowing while UniFi
     recording sat frozen. The heartbeat surfaces this as
     `dpackets=0` on the affected track immediately.
  2. Multi-minute gaps with no log activity at all were ambiguous:
     was nothing happening, or was something happening silently?
     The `stopProducers` decision trace + the consumer-removed
     remaining count surface the decision branches that were
     previously implicit.
- **No new tests:** Both additions are diagnostic logs at debug/trace
  level with no behavior change; covered by existing tests
  (compilation + `TestKickConsumers`).

### 14. Structured logging for Nest OAuth/extend lifecycle

- **Files:** `pkg/nest/api.go`, `pkg/nest/client.go`,
  `internal/nest/init.go`
- **Change:** Added a package-level `nest.Log` callback (default
  no-op so `pkg/nest` stays importable without a logging
  dependency) and wired it up from `internal/nest/init.go` to the
  application zerolog instance. Used to surface the previously-
  silent OAuth/extend lifecycle:
  - `[nest] OAuth cache hit` / `OAuth acquiring new token` (debug)
  - `[nest] OAuth token acquired expires_at=... ttl=...` (info)
  - `[nest] OAuth token request rejected status=...` (error)
  - `[nest] OAuth token refresh starting previous_expires_at=...
    since_predicted_expiry=...` (debug)
  - `[nest] OAuth refresh transport error` / `OAuth refresh
    rejected status=...` (error)
  - `[nest] OAuth token refreshed new_expires_at=... new_ttl=...`
    (info)
  - `[nest] extend timer armed expires_at=... next_fire_in=...
    session=...` (debug)
  - `[nest] extend ok expires_at=... next_fire_in=... session=...`
    (debug)
  - `[nest] extend failed, retrying in 10s` (warn)
  - `[nest] extend giving up — stream will die at expires_at`
    (error)
  - `[nest] extend got 401, refreshing OAuth token
    predicted_expires_at=... until_predicted_expiry=...` (info) —
    sign of `until_predicted_expiry` distinguishes "Google rotated
    the token earlier than we predicted" (positive) from "we let
    the token lapse past our own prediction" (negative, expected
    on the lazy 60-min boundary)
  - `[nest] extend timer cancelled` / `extend goroutine stopped`
    (debug)
  - `[nest] refresh timer fired — triggering proactive reconnect
    age=55m0s session=...` (info)
- **Why:** Before this, both successful extends and OAuth refreshes
  were completely silent, and failure cases produced only a generic
  error string with no context — making it impossible to tell from
  logs alone whether extends were firing, whether a 401 was being
  refreshed, or whether the refresh itself was failing. With this
  logging, any OAuth/extend issue is diagnosable from a single
  filtered pass through the log.

---

## Core / RTP

### 15. RTP continuity rewriting across upstream session swaps

- **Files:** `pkg/core/track.go`, `pkg/core/track_test.go`,
  `pkg/core/helpers.go`
- **Change:** `core.Receiver` now rewrites outgoing RTP packets so
  downstream consumers see a single continuous stream across
  upstream session swaps (`Receiver.Replace()`, triggered by
  `Producer.reconnect()` — including the proactive reconnect on
  lifetime-limited sources, section 12b). Specifically:
  - **Stable outgoing SSRC.** Each `Receiver` picks a random 32-bit
    SSRC at creation; that SSRC stays constant for the receiver's
    lifetime and is transferred to successor receivers in
    `Receiver.Replace()`. Downstream consumers see one SSRC for
    the entire duration of their consumer-side session.
  - **Continuous sequence numbers.** On detected upstream SSRC
    change, the first packet of the new session is rewritten to
    `lastOutgoingSeq + 1`. Subsequent packets follow the new
    session's relative spacing, so jitter buffers see a
    monotonically advancing sequence space.
  - **Forward-advancing timestamps.** On the same boundary, the
    first packet's timestamp is rewritten to `lastOutgoingTS +
    (codec.ClockRate × wall-clock elapsed)`, clamped to a sane
    range. This preserves codec-rate spacing across the swap
    rather than producing a backwards jump or a zero-duration
    burst.
  - **No-op for non-RTP codecs.** Gated by `codec.IsRTP()`; raw-
    payload pipelines (`PayloadTypeRAW`) are untouched.
  - **Test coverage:** `TestReceiverRTPContinuity` (5 sub-tests:
    non-RTP passthrough, single-session passthrough, mid-stream
    SSRC change anchoring, relative-spacing preservation after a
    swap, Replace state transfer to successor).
  - **Drive-by:** added missing `StripUserinfo` helper that had a
    test (`TestStripUserinfo`) but no implementation; was blocking
    the `pkg/core` test build.
- **Why:** UniFi Protect 7.1.69's RTSP recording subsystem treats an
  SSRC change mid-session as "the stream I was recording has
  ended" and stops writing to disk — even though the RTSP/TCP
  session stays alive and packets keep arriving. Without the
  rewrite, every proactive reconnect (section 12b) silently stops
  UniFi recording at the moment of the SSRC change. With the
  rewrite, the proactive reconnect is invisible at the RTP layer
  and UniFi keeps writing straight through. Validated against
  several hours of continuous recording across multiple proactive
  reconnects.
- **Why at the Receiver level, not per-Sender:** All consumers of a
  producer's track see the same Receiver. Rewriting once in the
  Receiver's `Input` closure means a single state machine handles
  every downstream consumer; per-Sender rewriting would require
  cloning each packet per consumer (allocation cost on every RTP
  packet) and per-consumer state machines. Trade-off: every
  consumer of a given Receiver sees the same outgoing SSRC. That's
  fine for typical setups (one upstream, several downstream
  consumers); if per-consumer SSRC ever becomes a requirement, the
  rewrite can be moved into the Sender path.

---

## Test coverage

Five new tests added in `pkg/onvif/onvif_test.go` covering the
ONVIF server-side changes:

- `TestGetPosixTZ` — pure-function test of the new POSIX TZ
  formatter. Covers UTC, west/east-of-UTC standard offsets,
  half-hour offsets (NST, IST), and the empty-zone-name fallback.
  Uses `time.FixedZone()` so test results are deterministic
  regardless of the host machine's TZ.
- `TestGetCapabilitiesResponse` — verifies all three services
  (Device, Media, Imaging) appear in the GetCapabilities response.
- `TestGetServicesResponse` — verifies all three WSDL namespaces
  and service URLs appear in the GetServices response.
- `TestGetProfilesResponseIncludesAudio` — regression guard for
  the AudioSourceConfiguration + AudioEncoderConfiguration blocks
  in `appendProfile`. Without these, UniFi Protect won't negotiate
  the audio track during RTSP SETUP.
- `TestImagingResponses` + `TestStaticResponseRoutesImagingOps`
  — verify imaging stubs use the `timg:` namespace and that the
  operation-name dispatcher routes correctly.

Additional tests added in this iteration:

- `pkg/core/track_test.go::TestReceiverRTPContinuity` (5 sub-tests)
  — covers the RTP rewriting (section 15): non-RTP passthrough,
  single-session passthrough, mid-stream SSRC change anchoring,
  relative-spacing preservation, and Replace state transfer to
  successor.
- `pkg/nest/api_test.go::TestExtendStreamRetry` (6 sub-tests) —
  covers the new ExtendStream retry loop (section 10c): happy
  path, 429-then-success, 409-then-success, retry exhaustion
  with three consecutive 429s, 5xx fail-fast (only 401/409/429
  retried), and backoff doubling between attempts. Uses an
  `httptest` server hooked in via the `extendURI` package var.
- `internal/streams/stream_test.go::TestKickConsumers` (6 sub-tests
  including internal-loop-skip and grace-period bump/release —
  see section 8b/8b-1).
- `internal/streams/stream_test.go::TestNewStreamLinksProducers`
  (3 sub-tests covering producer back-reference wiring).
- `internal/streams/stream_test.go::TestRedactSourceURL` (6 sub-
  tests covering nest, rtsp+auth, ffmpeg, fragment-only, empty).

No new tests added for the OAuth refresh path (section 10b),
proactive reconnect wiring (section 12b), or per-producer
heartbeat (section 13) — these touch goroutine/network paths
that need heavy mocking to exercise, and are validated by the
end-to-end integration testing the debugging session amounted to
(6+ hours of continuous operation across multiple proactive
reconnects + OAuth boundaries). All existing tests in
`pkg/onvif/onvif_test.go`, `pkg/rtsp/rtsp_test.go`,
`pkg/rtsp/client_test.go`, `pkg/core/track_test.go` continue to
pass.

## Build verification

All changes compile under Go 1.25 with no new dependencies. Built and
deployed continuously through the debugging session against the
production go2rtc Docker image (`docker/Dockerfile`, multi-stage build).

---

## CI / Build

### 16. Weekly scheduled rebuild for CVE refresh

- **Files:** `.github/workflows/build.yml`
- **Change:** The Docker-image workflow gains two coupled additions:
  - A `schedule:` trigger (`cron: '0 4 * * 0'` — Sunday 04:00 UTC)
    alongside the existing `workflow_dispatch` and `push` triggers,
    so the published image is rebuilt weekly even when no commits
    land.
  - `pull: true` on the `docker/build-push-action@v6` step so the
    base layers (`FROM golang:${GO_VERSION}-alpine` and
    `FROM python:${PYTHON_VERSION}-alpine`) re-resolve against the
    registry on every build. Without this the GitHub-Actions layer
    cache can indefinitely re-use a vulnerable `apk add` layer
    because the cache key matches the unchanged instruction text;
    `pull: true` bumps the parent layer SHA whenever Alpine ships
    a refreshed base image, which invalidates downstream cache and
    forces `apk` to re-install at current package versions.
- **Why:** Tagged releases of go2rtc are infrequent. Between
  releases, the `master` and `latest` images' OS package set was
  frozen at the last code push to `master`, while Alpine ships
  CVE-fix updates constantly. Combining the weekly rebuild with
  `pull: true` gives a predictable CVE-refresh cadence without
  needing a code change.
- **Source:** Pattern adopted from open upstream pull request
  https://github.com/AlexxIT/go2rtc/pull/2242. The fork's
  workflow is Docker-only (no `build-binaries` job), so the
  trade-off the upstream PR's author noted does not apply here.
- **No code change, no test impact.**
