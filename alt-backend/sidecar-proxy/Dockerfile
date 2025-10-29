# Dockerfile for lightweight proxy sidecar
# Multi-stage build optimized for minimal size and security
# Following ISSUE_RESOLVE_PLAN.md specifications for lightweight deployment

# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o proxy-sidecar \
    ./cmd/main.go

# Runtime stage - minimal distroless image
FROM gcr.io/distroless/static:nonroot

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy CA certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary
COPY --from=builder /build/proxy-sidecar /proxy-sidecar

# Set non-root user (distroless nonroot user)
USER nonroot:nonroot

# Expose ports
EXPOSE 8080 8081 9090

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/proxy-sidecar", "-health-check"] || exit 1

# Set entrypoint
ENTRYPOINT ["/proxy-sidecar"]

# Labels for container metadata
LABEL maintainer="Alt RSS Team" \
      version="1.0.0" \
      description="Lightweight proxy sidecar for upstream resolution" \
      purpose="Transform upstream=internal-ip to upstream=external-domain" \
      architecture="sidecar-pattern" \
      documentation="ISSUE_RESOLVE_PLAN.md"