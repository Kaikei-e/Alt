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
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy the binary
COPY --from=builder /build/auth-hub /auth-hub

# Expose port
EXPOSE 8888

# Run the application
ENTRYPOINT ["/auth-hub"]
