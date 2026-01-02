# ==============================================================================
# ビルドステージ: 依存関係キャッシュ
# ==============================================================================
FROM denoland/deno:2.6.3 AS builder

WORKDIR /app

# 依存関係ファイルを先にコピー
COPY deno.json deno.lock* ./
COPY src/ ./src/
COPY main.ts ./

# 依存関係をキャッシュ
RUN deno cache --lock=deno.lock main.ts || deno cache main.ts

# ==============================================================================
# ランタイムステージ: distroless + 非rootユーザー
# ==============================================================================
FROM gcr.io/distroless/cc:nonroot

# Deno バイナリをコピー
COPY --from=denoland/deno:2.6.3 /bin/deno /bin/deno

# キャッシュディレクトリをコピー
COPY --from=builder /deno-dir /deno-dir
COPY --from=builder /app /app

WORKDIR /app

# 環境変数
ENV DENO_DIR=/deno-dir \
    TOKEN_STORAGE_PATH=/app/secrets/oauth2_token.env

USER nonroot:nonroot

# 最小権限で実行 (--allow-all を廃止)
# /run/secrets: Docker secrets へのアクセス
ENTRYPOINT ["/bin/deno", "run", \
    "--allow-net", \
    "--allow-read=/app,/run/secrets", \
    "--allow-write=/app/secrets", \
    "--allow-env", \
    "main.ts", "daemon"]