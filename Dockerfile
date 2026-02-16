# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /axiom-docker-logger \
    .

# Rootfs stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates
RUN mkdir -p /run/docker/plugins

COPY --from=builder /axiom-docker-logger /axiom-docker-logger
