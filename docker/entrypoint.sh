#!/bin/sh
# Entrypoint wrapper for go2rtc Docker container.
# If /var/www/go2rtc exists (CDN files bundled), automatically
# configures static_dir to serve web UI without internet access.
if [ -d /var/www/go2rtc ]; then
    exec go2rtc \
        -config '{"api":{"static_dir":"/var/www/go2rtc"}}' \
        -config /config/go2rtc.yaml \
        "$@"
else
    exec go2rtc -config /config/go2rtc.yaml "$@"
fi
