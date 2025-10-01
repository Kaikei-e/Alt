# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Copy go mod files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags='-w -s -extldflags "-static"' \
    -o auth-hub main.go

# Final stage
FROM scratch

# Copy CA certificates for HTTPS requests to Kratos
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy the binary
COPY --from=builder /build/auth-hub /auth-hub

# Expose port
EXPOSE 8888

# Note: Health check will be configured in docker-compose.yml
# using wget/curl against the /health endpoint

# Run the application
ENTRYPOINT ["/auth-hub"]

# Note: Not using USER directive with scratch image
# The binary is statically compiled and runs with minimal privileges
