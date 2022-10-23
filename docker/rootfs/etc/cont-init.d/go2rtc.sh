#!/command/with-contenv bashio
# shellcheck shell=bash

bashio::log.info 'Preparing go2rtc log folder...'

mkdir -p /var/log/go2rtc
chown nobody:nogroup /var/log/go2rtc
chmod 02755 /var/log/go2rtc
