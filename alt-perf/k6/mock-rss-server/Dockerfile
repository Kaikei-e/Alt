FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS builder

WORKDIR /build
COPY go.mod .
COPY main.go .
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o mock-rss-server .

FROM alpine:3.21
RUN apk add --no-cache wget \
  && adduser -D -u 10001 -H nonroot
COPY --from=builder /build/mock-rss-server /mock-rss-server
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/mock-rss-server"]
