# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY app/go.mod app/go.sum* ./

# Download dependencies
RUN go mod download

# Copy source code
COPY app/ ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /mq-hub main.go

# Runtime stage
FROM alpine:3.20

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 app && adduser -u 1000 -G app -s /bin/sh -D app

WORKDIR /app

# Copy binary from builder
COPY --from=builder /mq-hub /mq-hub

# Switch to non-root user
USER app

# Expose port
EXPOSE 9500

# Health check
HEALTHCHECK --interval=10s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:9500/health || exit 1

# Run the binary
ENTRYPOINT ["/mq-hub"]
