FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod .
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o otel-relay .

FROM scratch
COPY --from=builder /app/otel-relay /otel-relay
ENTRYPOINT ["/otel-relay"]
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/otel-relay", "healthcheck"]
