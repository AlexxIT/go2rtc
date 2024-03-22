# syntax=docker/dockerfile:1.4

# 0. Prepare images
# only debian 13 (trixie) has latest ffmpeg
# https://packages.debian.org/trixie/ffmpeg
ARG DEBIAN_VERSION="trixie-slim"
ARG GO_VERSION="1.21-bookworm"
ARG NGROK_VERSION="3"

FROM --platform=linux/amd64 debian:${DEBIAN_VERSION} AS base
FROM golang:${GO_VERSION} AS go
FROM --platform=linux/amd64 ngrok/ngrok:${NGROK_VERSION} AS ngrok


# 1. Build go2rtc binary
FROM --platform=$BUILDPLATFORM go AS build
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go install -ldflags "-s -w" -trimpath github.com/mikefarah/yq/v4@v4.42.1

# 2. Collect all files
FROM scratch AS rootfs

COPY --link --from=build /build/go2rtc /go/bin/yq /usr/local/bin/
COPY --link --from=ngrok /bin/ngrok /usr/local/bin/

# 3. Final image
FROM base
# Prepare apt for buildkit cache
RUN rm -f /etc/apt/apt.conf.d/docker-clean \
  && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' >/etc/apt/apt.conf.d/keep-cache
# Install ffmpeg, bash (for run.sh), tini (for signal handling),
# and other common tools for the echo source.
# non-free for Intel QSV support (not used by go2rtc, just for tests)
# mesa-va-drivers for AMD APU
# libasound2-plugins for ALSA support
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked --mount=type=cache,target=/var/lib/apt,sharing=locked \
    echo 'deb http://deb.debian.org/debian trixie non-free' > /etc/apt/sources.list.d/debian-non-free.list && \
    apt-get -y update && apt-get -y install tini ffmpeg \
        python3 curl jq \
        intel-media-va-driver-non-free \
        mesa-va-drivers \
        libasound2-plugins

COPY --link --from=rootfs / /
COPY --chmod=755 entrypoint.sh /entrypoint.sh

ENTRYPOINT ["/usr/bin/tini", "--", "/entrypoint.sh"]
VOLUME /config
WORKDIR /config
# https://github.com/NVIDIA/nvidia-docker/wiki/Installation-(Native-GPU-Support)
ENV NVIDIA_VISIBLE_DEVICES all
ENV NVIDIA_DRIVER_CAPABILITIES compute,video,utility