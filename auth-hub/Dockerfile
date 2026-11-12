# Build stage
FROM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS builder

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
