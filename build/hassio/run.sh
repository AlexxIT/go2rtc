#!/usr/bin/with-contenv bashio

set +e

cd /config

while true; do
    if [ -x go2rtc ]; then
        ./go2rtc
    else
        /app/go2rtc
    fi

    sleep 5
done