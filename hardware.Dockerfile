# 0. Prepare images
# only debian 12 (bookworm) has latest ffmpeg
ARG DEBIAN_VERSION="bookworm-slim"
ARG GO_VERSION="1.19-buster"
ARG NGROK_VERSION="3"

FROM debian:${DEBIAN_VERSION} AS base
FROM golang:${GO_VERSION} AS go
FROM ngrok/ngrok:${NGROK_VERSION} AS ngrok

# 0. collect ace editor
FROM alpine:latest as ace
RUN apk add curl
RUN <<EOT
    for i in \
        https://cdn.jsdelivr.net/npm/ace-builds@1.14.0/src-min-noconflict/ace.min.js \
        https://cdn.jsdelivr.net/npm/ace-builds@1.14.0/src-min-noconflict/mode-yaml.min.js \
        https://cdn.jsdelivr.net/npm/ace-builds@1.14.0/src-min-noconflict/worker-yaml.min.js \
        https://cdn.jsdelivr.net/npm/ace-builds@1.14.0/src-min-noconflict/theme-terminal.min.js \
        https://cdn.jsdelivr.net/npm/ace-builds@1.14.0/src-min-noconflict/theme-monokai.min.js
    do
        curl -sLk "$i" >> /ace.js; echo "" >> /ace.js;
    done
EOT

# 1. Build go2rtc binary
FROM go AS build

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=ace /ace.js www/ace.js
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath


# 2. Collect all files
FROM scratch AS rootfs

COPY --from=build /build/go2rtc /usr/local/bin/
COPY --from=ngrok /bin/ngrok /usr/local/bin/
COPY ./build/docker/run.sh /


# 3. Final image
FROM base

# Install ffmpeg, bash (for run.sh), tini (for signal handling),
# and other common tools for the echo source.
# non-free for Intel QSV support (not used by go2rtc, just for tests)
RUN echo 'deb http://deb.debian.org/debian bookworm non-free' > /etc/apt/sources.list.d/debian-non-free.list && \
    apt-get -y update && apt-get -y install tini ffmpeg python3 curl jq intel-media-va-driver-non-free

COPY --from=rootfs / /

RUN chmod a+x /run.sh && mkdir -p /config

ENTRYPOINT ["/usr/bin/tini", "--"]

# https://github.com/NVIDIA/nvidia-docker/wiki/Installation-(Native-GPU-Support)
ENV NVIDIA_VISIBLE_DEVICES all
ENV NVIDIA_DRIVER_CAPABILITIES compute,video,utility

CMD ["/run.sh"]
