FROM golang:1.23.1
ENV TZ=Europe/Moscow
ARG CGO_ENABLED=0
WORKDIR /app
COPY . .
RUN apt-get update && apt-get install -y ca-certificates
RUN go build
ENTRYPOINT ["/schedule-bot"]
