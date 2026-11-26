FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS builder

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
