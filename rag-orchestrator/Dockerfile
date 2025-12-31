FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go.mod files first (including proto module for replace directive)
COPY go.mod go.sum ./
COPY internal/gen/proto/go.mod internal/gen/proto/go.sum* ./internal/gen/proto/

RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o rag-orchestrator cmd/server/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/rag-orchestrator .

EXPOSE 9010
EXPOSE 9011
CMD ["./rag-orchestrator"]
