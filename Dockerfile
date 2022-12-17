# 0. Prepare images
ARG PYTHON_VERSION="3.11"
ARG GO_VERSION="1.19"
ARG NGROK_VERSION="3"

FROM python:${PYTHON_VERSION}-alpine AS base
FROM golang:${GO_VERSION}-alpine AS go
FROM ngrok/ngrok:${NGROK_VERSION}-alpine AS ngrok


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

COPY --from=build /build/go2rtc /usr/local/bin/
COPY --from=ngrok /bin/ngrok /usr/local/bin/
COPY ./build/docker/run.sh /


# 3. Final image
FROM base

# Install ffmpeg, bash (for run.sh), tini (for signal handling),
# and other common tools for the echo source.
RUN apk add --no-cache tini ffmpeg bash curl jq

# Hardware Acceleration for Intel CPU (+50MB)
ARG TARGETARCH

RUN if [ "${TARGETARCH}" = "amd64" ]; then apk add --no-cache libva-intel-driver intel-media-driver; fi

# Hardware: AMD and NVidia VAAPI (not sure about this)
# RUN libva-glx mesa-va-gallium
# Hardware: AMD and NVidia VDPAU (not sure about this)
# RUN libva-vdpau-driver mesa-vdpau-gallium (+150MB total)

COPY --from=rootfs / /

RUN chmod a+x /run.sh && mkdir -p /config

ENTRYPOINT ["/sbin/tini", "--"]

CMD ["/run.sh"]
