# https://hub.docker.com/_/python/tags?page=1&name=-alpine
ARG PYTHON_VERSION="3.10.8"
# https://hub.docker.com/_/golang/tags?page=1&name=-alpine
ARG GO_VERSION="1.19.3"
# https://hub.docker.com/r/ngrok/ngrok/tags?page=1&name=-alpine
ARG NGROK_VERSION="3.1.0"


FROM python:${PYTHON_VERSION}-alpine AS base


FROM golang:${GO_VERSION}-alpine AS go


FROM ngrok/ngrok:${NGROK_VERSION}-alpine AS ngrok


# Build go2rtc binary
FROM go AS build

WORKDIR /workspace

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build binary
COPY cmd cmd
COPY pkg pkg
COPY www www
COPY main.go .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath

RUN mkdir -p /config

# Collect all files
FROM scratch AS rootfs

COPY --from=build /workspace/go2rtc /usr/local/bin/
# Ensure an empty /config folder exists so that the container can be run without a volume
COPY --from=build /config /config
COPY --from=ngrok /bin/ngrok /usr/local/bin/
COPY ./docker/run.sh /run.sh


# Final image
FROM base

# Install ffmpeg, bash (for run.sh), tini (for signal handling),
# and other common tools for the echo source.
RUN apk add --no-cache ffmpeg bash tini curl jq

COPY --from=rootfs / /

ENTRYPOINT ["/sbin/tini", "--"]

CMD ["/run.sh"]
