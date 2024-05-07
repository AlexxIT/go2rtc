#!/bin/sh

CONFIG_PATH=${CONFIG_PATH:-/config/go2rtc.yaml}
PORT=${PORT:-1984}

# Create the config file with default settings if it doesn't exist
if [ ! -f "$CONFIG_PATH" ]; then
    mkdir -p "$(dirname "$CONFIG_PATH")"
    echo "api:" > "$CONFIG_PATH"
    echo -e "\tlisten: ':$PORT'" >> "$CONFIG_PATH"
fi

exec go2rtc -config "$CONFIG_PATH" "$@"
