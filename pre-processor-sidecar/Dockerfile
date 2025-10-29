# Multi-stage build for pre-processor-sidecar OAuth2-enabled CronJob
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY app/go.mod app/go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY app/ ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o pre-processor-sidecar ./cmd/main.go

# Final stage
FROM alpine:3.21

# Install CA certificates
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Create non-root user
RUN addgroup -g 1000 -S appgroup && \
    adduser -u 1000 -S appuser -G appgroup

# Copy binary from builder stage
COPY --from=builder /app/pre-processor-sidecar ./pre-processor-sidecar

# Change ownership to appuser
RUN chown appuser:appgroup pre-processor-sidecar && \
    chmod +x pre-processor-sidecar

# Switch to non-root user
USER appuser

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD /app/pre-processor-sidecar --health-check

# Default command
ENTRYPOINT ["/app/pre-processor-sidecar"]