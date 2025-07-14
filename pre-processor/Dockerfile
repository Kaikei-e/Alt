FROM golang:1.24.3-alpine AS builder

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

# Copy the binary from builder stage
COPY --from=builder /app/pre-processor .

EXPOSE 9200

CMD ["./pre-processor"]