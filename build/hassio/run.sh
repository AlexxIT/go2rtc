#!/usr/bin/with-contenv bashio

set +e

while true; do
    if [ -x /config/go2rtc ]; then
        /config/go2rtc -config /config/go2rtc.yaml
    else
        /app/go2rtc -config /config/go2rtc.yaml
    fi

    sleep 5
done