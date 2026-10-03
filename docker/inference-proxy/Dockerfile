FROM golang:1.27.1-alpine AS builder
WORKDIR /app
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -o inference-proxy main.go

FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/inference-proxy /inference-proxy
ENTRYPOINT ["/inference-proxy"]
