FROM golang:1.24-alpine AS builder

WORKDIR /build
COPY go.mod .
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o mock-rss-server .

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=builder /build/mock-rss-server /mock-rss-server
EXPOSE 8080
ENTRYPOINT ["/mock-rss-server"]
