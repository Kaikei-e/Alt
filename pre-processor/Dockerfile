FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install git for go mod download
RUN apk add --no-cache git

# Copy go mod files first for better caching
COPY ./app/go.mod ./app/go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY ./app ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o pre-processor .

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder stage with executable permissions
COPY --from=builder /app/pre-processor .
RUN chmod +x pre-processor

EXPOSE 9200

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD ./pre-processor --health-check || exit 1

CMD ["./pre-processor"]