FROM golang:1.23.1 as builder
ARG CGO_ENABLED=0
WORKDIR /app
COPY . .
RUN go build

FROM scratch
COPY --from=builder /app/schedule-bot /schedule-bot
ENTRYPOINT ["/schedule-bot"]
