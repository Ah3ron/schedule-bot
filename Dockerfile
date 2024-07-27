FROM golang:alpine3.20

WORKDIR /app

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o /out

CMD ["/out"]