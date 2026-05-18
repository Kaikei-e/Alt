FROM golang:1.26-alpine@sha256:91eda9776261207ea25fd06b5b7fed8d397dd2c0a283e77f2ab6e91bfa71079d AS builder

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
FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639

COPY --from=builder /app/rag-orchestrator /rag-orchestrator
COPY --from=builder /app/backfill /backfill

USER nonroot:nonroot

EXPOSE 9010
EXPOSE 9011

ENTRYPOINT ["/rag-orchestrator"]
