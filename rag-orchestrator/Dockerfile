FROM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS builder

WORKDIR /app

# go.modファイルをコピー (proto モジュールのreplace directive含む)
COPY go.mod go.sum ./
COPY internal/gen/proto/go.mod internal/gen/proto/go.sum* ./internal/gen/proto/

RUN go mod download

COPY . .

# 最適化ビルド: シンボル除去 + 静的リンク
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o rag-orchestrator cmd/server/main.go
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o backfill cmd/backfill/main.go

# 最小イメージ (distroless/static) + 非rootユーザー
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

COPY --from=builder /app/rag-orchestrator /rag-orchestrator
COPY --from=builder /app/backfill /backfill

USER nonroot:nonroot

EXPOSE 9010
EXPOSE 9011

ENTRYPOINT ["/rag-orchestrator"]
