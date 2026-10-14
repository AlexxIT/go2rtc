# Home Assistant add-on: official go2rtc image with the binary of this fork.
# The Supervisor only sends this folder as build context, so the source is
# cloned here. For a new version change GO2RTC_REF and "version" in config.yaml.
FROM golang:1.26-alpine AS build
RUN apk add --no-cache git
ARG GO2RTC_REPO=https://git.btaubmann.de/tux/go2rtc.git
ARG GO2RTC_REF=e3663d915d9a6e6450696884e7fb0047646453ab
RUN git clone ${GO2RTC_REPO} /src \
    && cd /src && git checkout ${GO2RTC_REF} \
    && CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath -o /go2rtc .

FROM alexxit/go2rtc:1.9.14
COPY --from=build /go2rtc /usr/local/bin/go2rtc
