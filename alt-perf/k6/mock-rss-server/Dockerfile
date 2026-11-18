FROM golang:1.27.3-alpine@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS builder

WORKDIR /build
COPY go.mod .
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o mock-rss-server .

FROM alpine:3.24
RUN apk add --no-cache wget \
  && adduser -D -u 10001 -H nonroot
COPY --from=builder /build/mock-rss-server /mock-rss-server
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/mock-rss-server"]
