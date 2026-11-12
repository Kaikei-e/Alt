FROM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS builder

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
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

COPY --from=builder /app/pre-processor /pre-processor

USER nonroot:nonroot

EXPOSE 9200

ENTRYPOINT ["/pre-processor"]