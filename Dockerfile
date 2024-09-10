FROM golang
ARG CGO_ENABLED=0
WORKDIR /app
COPY . .
RUN go build
CMD ["/schedule-bot"]
