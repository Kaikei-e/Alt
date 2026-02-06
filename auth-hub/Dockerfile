# Build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /build

# Cache dependency downloads
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Optimized build: strip symbols + static linking
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o auth-hub ./cmd/auth-hub

# Minimal image (distroless/static) + non-root user
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /build/auth-hub /auth-hub

USER nonroot:nonroot

EXPOSE 8888

ENTRYPOINT ["/auth-hub"]
