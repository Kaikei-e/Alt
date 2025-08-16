# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o auth-service ./app/cmd/server

# Build the migration tool
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o migrate ./app/cmd/migrate

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata curl

# Create app directory
WORKDIR /app

# Copy binaries from builder stage
COPY --from=builder /app/auth-service .
COPY --from=builder /app/migrate .

# Copy configuration files if needed
COPY --from=builder /app/app/config ./config

# Create non-root user
RUN addgroup -g 1001 -S authservice && \
    adduser -S -D -H -u 1001 -h /app -s /sbin/nologin -G authservice -g authservice authservice

# Change ownership of app directory
RUN chown -R authservice:authservice /app

# Switch to non-root user
USER authservice

# Expose port
EXPOSE 9500

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:9500/health || exit 1

# Run the application
CMD ["./auth-service"]