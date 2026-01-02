FROM golang:1.25-alpine AS builder

WORKDIR /app

# 依存関係キャッシュ最適化
COPY ./app/go.mod ./app/go.sum ./
RUN go mod download

COPY ./app ./

# 最適化ビルド: シンボル除去 + 静的リンク
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o pre-processor .

# 最小イメージ (distroless/static) + 非rootユーザー
# CA証明書込み、シェルなし
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /app/pre-processor /pre-processor

USER nonroot:nonroot

EXPOSE 9200

ENTRYPOINT ["/pre-processor"]