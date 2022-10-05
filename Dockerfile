# https://github.com/hassio-addons/addon-base-python/releases
ARG BASE_VERSION="9.0.1"
# https://hub.docker.com/_/golang/tags?page=1&name=-alpine
ARG GO_VERSION="1.19.2"
# https://hub.docker.com/r/ngrok/ngrok/tags?page=1&name=-alpine
ARG NGROK_VERSION="3.1.0"


FROM ghcr.io/hassio-addons/base-python:${BASE_VERSION} AS base


FROM golang:${GO_VERSION}-alpine AS go


FROM ngrok/ngrok:${NGROK_VERSION}-alpine AS ngrok


# Build go2rtc binary
FROM go AS build

WORKDIR /workspace

COPY . .

RUN CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath


# Collect all files
FROM scratch AS rootfs

COPY --from=build /workspace/go2rtc /usr/local/bin/
COPY --from=ngrok /bin/ngrok /usr/local/bin/
COPY ./docker/rootfs/ /


# Final stage
FROM base

# Install ffmpeg
RUN apk add --no-cache ffmpeg

COPY --from=rootfs / /
