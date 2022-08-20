ARG BUILD_FROM
FROM $BUILD_FROM

RUN apk add --no-cache git go

WORKDIR app

RUN git clone https://github.com/AlexxIT/go2rtc .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath

CMD [ "/app/go2rtc", "-config", "/config/go2rtc.yaml" ]
