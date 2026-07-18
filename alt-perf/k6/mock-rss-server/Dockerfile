FROM golang:1.26-alpine@sha256:91eda9776261207ea25fd06b5b7fed8d397dd2c0a283e77f2ab6e91bfa71079d AS builder

WORKDIR /build
COPY go.mod .
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o mock-rss-server .

FROM alpine:3.21
RUN apk add --no-cache wget \
  && adduser -D -u 10001 -H nonroot
COPY --from=builder /build/mock-rss-server /mock-rss-server
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/mock-rss-server"]
