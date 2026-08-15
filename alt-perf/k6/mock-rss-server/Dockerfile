FROM golang:1.26-alpine@sha256:70b46548e42db77e0966aaf3619fd068734dc6c77584d526b91126504fd95816 AS builder

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
