# syntax = docker/dockerfile-upstream:master-labs
# 0. Prepare images
# only debian 12 (bookworm) has latest ffmpeg
ARG DEBIAN_VERSION="bookworm-slim"
ARG GO_VERSION="1.19-buster"
ARG NGROK_VERSION="3"

FROM debian:${DEBIAN_VERSION} AS base
FROM golang:${GO_VERSION} AS go
FROM ngrok/ngrok:${NGROK_VERSION} AS ngrok


# 1. Build go2rtc binary
FROM go AS build

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath


# 2. Collect all files
FROM scratch AS rootfs

COPY --link --from=build /build/go2rtc /usr/local/bin/
COPY --link --from=ngrok /bin/ngrok /usr/local/bin/
COPY ./build/docker/run.sh /


# 3. Final image
FROM base
ENV DEBIAN_FRONTEND=noninteractive

# Install ffmpeg, bash (for run.sh), tini (for signal handling),
# and other common tools for the echo source.
# non-free for Intel QSV support (not used by go2rtc, just for tests)
RUN --mount=type=cache,target=/var/apt/cache --mount=type=tmpfs,target=/tmp <<EOT
    apt update --allow-insecure-repositories && apt install -y --no-install-recommends ca-certificates software-properties-common
    update-ca-certificates
    apt-add-repository contrib && apt-add-repository non-free
    apt update && apt -y install tini python3 curl xz-utils jq intel-media-va-driver-non-free
    mkdir -p /usr/lib/btbn-ffmpeg
    curl -Ls -o btbn-ffmpeg.tar.xz "https://github.com/BtbN/FFmpeg-Builds/releases/download/autobuild-2022-07-31-12-37/ffmpeg-n5.1-2-g915ef932a3-linux64-gpl-shared-5.1.tar.xz"
    tar -xf btbn-ffmpeg.tar.xz -C /usr/lib/btbn-ffmpeg --strip-components 1
    rm -rf btbn-ffmpeg.tar.xz FFmpeg-Builds /usr/lib/btbn-ffmpeg/doc /usr/lib/btbn-ffmpeg/bin/ffplay
    chmod +x /usr/lib/btbn-ffmpeg/bin/*

    apt purge gnupg apt-transport-https wget xz-utils -y
    apt clean autoclean -y
    apt autoremove --purge -y
    rm -rf /var/lib/apt/lists/*
EOT

COPY --link --from=rootfs / /

RUN chmod a+x /run.sh && mkdir -p /config

ENTRYPOINT ["/usr/bin/tini", "--"]

# https://github.com/NVIDIA/nvidia-docker/wiki/Installation-(Native-GPU-Support)
ENV NVIDIA_VISIBLE_DEVICES all
ENV NVIDIA_DRIVER_CAPABILITIES compute,video,utility
ENV PATH="/usr/lib/btbn-ffmpeg/bin:${PATH}"

CMD ["/run.sh"]
