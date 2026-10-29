# Dedicated HomeKit listener

By default, HomeKit shares the HTTP API listener. An omitted or empty
`homekit_server.listen` keeps that behavior.

## Configuration

```yaml
api:
  listen: "127.0.0.1:1984"

homekit_server:
  listen: ":1985"

homekit:
  aqara1:  # must match an existing stream
    pin: "${HOMEKIT_PIN}"  # set this environment variable to a private 8-digit PIN
```

The separate listener exposes only HomeKit pairing and session verification.
The web UI, snapshots, and administrative HomeKit API stay on the API listener.
API authentication, `base_path`, and `allow_paths` do not apply to the HomeKit
listener. The API can also be disabled with `api.listen: ""`.

Keep the `homekit` module enabled when restricting `app.modules`.
`homekit_server` is a configuration section, not a module name.

## Pairing and migration

Unpaired cameras on the dedicated listener require a private PIN; the public
default is rejected. On both the shared and dedicated listeners, initial PIN
pairing closes after the first successful setup. Add other controllers through
the paired Apple Home, or remove all pairings to commission again. A migrated
camera with the public default PIN can still verify existing controllers, but
needs a private PIN to pair again.

Keep existing stream names, device identities, private keys, and `pairings`
when switching listeners. Changing the listener does not change them.

## Networking

All exported cameras share the port advertised through mDNS. Apple devices must
be able to discover the cameras and reach that TCP port. A bind failure logs an
error and prevents camera advertisement; there is no fallback to the API port.

A specific IPv4 bind restricts camera advertisements to that local address.
Wildcard binds use the matching local IPv4 interfaces. Specific IPv6 addresses
are rejected because discovery currently publishes IPv4 addresses only.

Camera media and HomeKit Data Stream traffic use separately negotiated
connections. This setting controls the HomeKit server; it does not change the
`homekit://` client or `srtp.listen`.
