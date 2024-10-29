FROM golang:1.23.1 AS builder
ARG CGO_ENABLED=0
WORKDIR /app
COPY . .
RUN go build -o schedule-bot .

FROM alpine:latest
RUN apk add --no-cache tzdata
ENV TZ=Europe/Minsk
RUN cp /usr/share/zoneinfo/$TZ /etc/localtime && \
    echo $TZ > /etc/timezone
WORKDIR /root/
COPY --from=builder /app/schedule-bot .
ENTRYPOINT ["./schedule-bot"]
