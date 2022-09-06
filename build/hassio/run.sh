#!/usr/bin/with-contenv bashio

set +e

# add the feature for update to any version
if [ -f "/config/go2rtc.version" ]; then
    branch=`cat /config/go2rtc.version`
    echo "Update to version $branch"
    git clone --depth 1 --branch "$branch" https://github.com/AlexxIT/go2rtc \
      && cd go2rtc \
      && CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath -o /usr/local/bin \
      && rm -r /go2rtc && rm /config/go2rtc.version
fi

# set cwd for go2rtc (for config file, Hass itegration, etc)
cd /config

# add the feature to override go2rtc binary from Hass config folder
export PATH="/config:$PATH"

while true; do
    go2rtc
    sleep 5
done