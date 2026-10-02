# Build stage
FROM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS builder

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o alt-butterfly-facade .

# Minimal distroless image with non-root user
FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639

COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

COPY --from=builder /build/alt-butterfly-facade /alt-butterfly-facade

USER nonroot:nonroot

EXPOSE 9200

ENTRYPOINT ["/alt-butterfly-facade"]
