#!/bin/bash

set -euo pipefail

echo "Starting go2rtc..." >&2

readonly config_path="/config"

if [[ -x "${config_path}/go2rtc" ]]; then
  readonly binary_path="${config_path}/go2rtc"
  echo "Using go2rtc binary from '${binary_path}' instead of the embedded one" >&2
else
  readonly binary_path="/usr/local/bin/go2rtc"
fi

# set cwd for go2rtc (for config file, Hass integration, etc)
cd "${config_path}" || echo "Could not change working directory to '${config_path}'" >&2

exec "${binary_path}"
