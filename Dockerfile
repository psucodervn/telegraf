ARG BINARY=main
ARG DIR=/app

FROM golang:1.14-alpine AS builder
ARG BINARY
ARG DIR

RUN apk update && apk add --no-cache git ca-certificates
WORKDIR $DIR

#COPY go.mod go.sum ./
#RUN go mod download
#
#COPY . .
#
#RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $BINARY main.go

COPY telegraf $DIR/$BINARY

FROM scratch
ARG BINARY
ARG DIR

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder $DIR/$BINARY ./app

ENTRYPOINT ["./app"]
