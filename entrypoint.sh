#!/bin/sh

CONFIG_PATH=${CONFIG_PATH:-/config/go2rtc.yaml}
PORT=${PORT:-1984}

# Check if yq is available
if ! command -v yq > /dev/null; then
    echo "yq command not found. Please install yq."
    exit 1
fi

# Create config file with default content if it doesn't exist
if [ ! -f "$CONFIG_PATH" ]; then
    mkdir -p "$(dirname "$CONFIG_PATH")"
    if ! yq -o yaml e -n ".api.listen = \":$PORT\"" > "$CONFIG_PATH"; then
        echo "Failed to create default config file at $CONFIG_PATH"
        exit 1
    fi
fi

exec go2rtc -config "$CONFIG_PATH" "$@"