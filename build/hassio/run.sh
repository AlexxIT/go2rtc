#!/usr/bin/with-contenv bashio

set +e

# set cwd for go2rtc (for config file, Hass itegration, etc)
cd /config

# add the feature to override go2rtc binary from Hass config folder
export PATH="/config:$PATH"

while true; do
    go2rtc
    sleep 5
done