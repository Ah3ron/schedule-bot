FROM golang:alpine3.20
ARG CGO_ENABLED=0
WORKDIR /app
COPY . .
RUN go build
ENTRYPOINT ["/schedule-bot"]
