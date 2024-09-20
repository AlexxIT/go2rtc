#!/bin/sh

CONFIG_PATH=${CONFIG_PATH:-/config/go2rtc.yaml}
PORT=${PORT:-1984}

if [ ! -f "$CONFIG_PATH" ]; then
    mkdir -p "$(dirname "$CONFIG_PATH")"
fi

exec go2rtc -config "{ api: { listen: \":$PORT\" } }" -config "$CONFIG_PATH" "$@"
