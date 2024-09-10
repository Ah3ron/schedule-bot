FROM golang:1.23.1 AS builder
ARG CGO_ENABLED=0
WORKDIR /app
COPY . .
RUN go build -o schedule-bot .

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/schedule-bot .
CMD ["./schedule-bot"]
