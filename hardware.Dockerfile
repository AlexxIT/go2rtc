# syntax=docker/dockerfile:labs

# 0. Prepare images
# only debian 12 (bookworm) has latest ffmpeg
ARG DEBIAN_VERSION="bookworm-slim"
ARG GO_VERSION="1.20-buster"
ARG NGROK_VERSION="3"

FROM debian:${DEBIAN_VERSION} AS base
FROM golang:${GO_VERSION} AS go
FROM ngrok/ngrok:${NGROK_VERSION} AS ngrok


# 1. Build go2rtc binary
FROM go AS build

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath


# 2. Collect all files
FROM scratch AS rootfs

COPY --link --from=build /build/go2rtc /usr/local/bin/
COPY --link --from=ngrok /bin/ngrok /usr/local/bin/

# 3. Final image
FROM base
# Prepare apt for buildkit cache
RUN rm -f /etc/apt/apt.conf.d/docker-clean \
  && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' >/etc/apt/apt.conf.d/keep-cache
# Install ffmpeg, bash (for run.sh), tini (for signal handling),
# and other common tools for the echo source.
# non-free for Intel QSV support (not used by go2rtc, just for tests)
# libasound2-plugins for ALSA support
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked --mount=type=cache,target=/var/lib/apt,sharing=locked \
    echo 'deb http://deb.debian.org/debian bookworm non-free' > /etc/apt/sources.list.d/debian-non-free.list && \
    apt-get -y update && apt-get -y install tini ffmpeg \
        python3 curl jq \
        intel-media-va-driver-non-free \
        libasound2-plugins

COPY --link --from=rootfs / /



ENTRYPOINT ["/usr/bin/tini", "--"]
VOLUME /config
WORKDIR /config
# https://github.com/NVIDIA/nvidia-docker/wiki/Installation-(Native-GPU-Support)
ENV NVIDIA_VISIBLE_DEVICES all
ENV NVIDIA_DRIVER_CAPABILITIES compute,video,utility

CMD ["go2rtc", "-config", "/config/go2rtc.yaml"]
