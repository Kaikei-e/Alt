FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS builder
WORKDIR /app
COPY go.mod .
COPY main.go .
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -o inference-proxy .

FROM gcr.io/distroless/static-debian12:nonroot
USER nonroot:nonroot
COPY --from=builder /app/inference-proxy /inference-proxy
ENTRYPOINT ["/inference-proxy"]
