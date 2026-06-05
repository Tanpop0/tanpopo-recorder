FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.work go.work.sum ./
COPY apps/gui apps/gui
COPY apps/cli apps/cli

RUN cd apps/cli && go build -o /out/twitcasting-recorder .
RUN cd apps/gui && go build -o /out/recorder-worker ./cmd/recorder-worker

FROM alpine:3.20

RUN apk add --no-cache ca-certificates ffmpeg tzdata
WORKDIR /app

COPY --from=build /out/twitcasting-recorder /usr/local/bin/twitcasting-recorder
COPY --from=build /out/recorder-worker /usr/local/bin/recorder-worker

VOLUME ["/config", "/download"]
ENTRYPOINT ["twitcasting-recorder"]
CMD ["monitor", "--config", "/config/config.yaml"]
