FROM golang:1.27.3-alpine@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS builder

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
FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639

COPY --from=builder /app/pre-processor /pre-processor

USER nonroot:nonroot

EXPOSE 9200

ENTRYPOINT ["/pre-processor"]