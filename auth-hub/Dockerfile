# Build stage
FROM golang:1.26-alpine@sha256:70b46548e42db77e0966aaf3619fd068734dc6c77584d526b91126504fd95816 AS builder

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
FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639

COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /build/auth-hub /auth-hub

USER nonroot:nonroot

EXPOSE 8888

ENTRYPOINT ["/auth-hub"]
