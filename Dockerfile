FROM golang:1.22-alpine3.20 AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go build -o nelm cmd/nelm/main.go

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/nelm /usr/local/bin/nelm

CMD ["nelm"]
