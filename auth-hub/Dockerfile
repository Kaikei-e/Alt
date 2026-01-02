# Build stage
FROM golang:1.25-alpine AS builder

# ビルド時に必要な依存関係
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /build

# 依存関係キャッシュ最適化
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 最適化ビルド: シンボル除去 + 静的リンク
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o auth-hub main.go

# 最小イメージ (distroless/static) + 非rootユーザー
FROM gcr.io/distroless/static-debian12:nonroot

# タイムゾーンデータをコピー
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

COPY --from=builder /build/auth-hub /auth-hub

USER nonroot:nonroot

EXPOSE 8888

ENTRYPOINT ["/auth-hub"]
