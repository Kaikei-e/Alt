FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o rag-orchestrator cmd/server/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/rag-orchestrator .

EXPOSE 9010
CMD ["./rag-orchestrator"]
