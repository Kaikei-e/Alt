FROM golang:1.26-alpine@sha256:70b46548e42db77e0966aaf3619fd068734dc6c77584d526b91126504fd95816 AS builder

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
