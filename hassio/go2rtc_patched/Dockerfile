# Home Assistant add-on: official go2rtc image with the binary of this fork.
# The Supervisor only sends this folder as build context, so the source is
# cloned here. For a new version change GO2RTC_REF and "version" in config.yaml.
FROM golang:1.26-alpine AS build
RUN apk add --no-cache git
ARG GO2RTC_REPO=https://git.btaubmann.de/tux/go2rtc.git
ARG GO2RTC_REF=650c9bba7d5e80d0adb895245925fb3f7de334e0
RUN git clone ${GO2RTC_REPO} /src \
    && cd /src && git checkout ${GO2RTC_REF} \
    && CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath -o /go2rtc .

FROM alexxit/go2rtc:1.9.14
COPY --from=build /go2rtc /usr/local/bin/go2rtc
