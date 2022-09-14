ARG BUILD_FROM

FROM $BUILD_FROM as build

# 1. Build go2rtc
RUN apk add --no-cache git go

RUN git clone https://github.com/AlexxIT/go2rtc \
    && cd go2rtc \
    && CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath

# 2. Download ngrok
ARG BUILD_ARCH

# https://github.com/home-assistant/docker-base/blob/master/alpine/Dockerfile
RUN if [ "${BUILD_ARCH}" = "aarch64" ]; then BUILD_ARCH="arm64"; \
    elif [ "${BUILD_ARCH}" = "armv7" ]; then BUILD_ARCH="arm"; fi \
    && cd go2rtc \
    && curl $(curl -s "https://raw.githubusercontent.com/ngrok/docker-ngrok/main/releases.json" | jq -r ".${BUILD_ARCH}.url") -o ngrok.zip \
    && unzip ngrok



# https://devopscube.com/reduce-docker-image-size/
FROM $BUILD_FROM

# 3. Copy go2rtc and ngrok to release
COPY --from=build /go2rtc/go2rtc /usr/local/bin
COPY --from=build /go2rtc/ngrok /usr/local/bin

# 4. Install ffmpeg
# apk base  OK: 22  MiB in 40  packages
# ffmpeg    OK: 113 MiB in 110 packages
# python3   OK: 161 MiB in 114 packages
RUN apk add --no-cache ffmpeg python3

# 5. Copy run to release
COPY run.sh /
RUN chmod a+x /run.sh

CMD [ "/run.sh" ]
