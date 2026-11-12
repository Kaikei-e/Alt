FROM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS builder

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
