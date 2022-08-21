ARG BUILD_FROM
FROM $BUILD_FROM

RUN apk add --no-cache git go ffmpeg

ARG BUILD_ARCH

WORKDIR app

RUN git clone https://github.com/AlexxIT/go2rtc .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath

# https://github.com/home-assistant/docker-base/blob/master/alpine/Dockerfile
RUN if [ "${BUILD_ARCH}" = "aarch64" ]; then BUILD_ARCH="arm64"; \
    elif [ "${BUILD_ARCH}" = "armv7" ]; then BUILD_ARCH="arm"; fi \
    && curl $(curl -s "https://raw.githubusercontent.com/ngrok/docker-ngrok/main/releases.json" | jq -r ".${BUILD_ARCH}.url") -o ngrok.zip \
    && unzip ngrok

COPY run.sh /
RUN chmod a+x /run.sh

CMD [ "/run.sh" ]
