# ==============================================================================
# ビルドステージ: 依存関係キャッシュ
# ==============================================================================
FROM denoland/deno:2.6.3 AS builder

WORKDIR /app

# 依存関係ファイルを先にコピー
COPY deno.json deno.lock* ./
COPY src/ ./src/
COPY main.ts ./

# 依存関係をキャッシュ (lockfile 不整合は fail-closed: 未検証の依存で
# 本番イメージを焼かない)
RUN deno cache --lock=deno.lock main.ts

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
    "--allow-net=www.inoreader.com:443,rask-log-aggregator:4318,localhost:4318,0.0.0.0:9201", \
    "--allow-read=/app,/run/secrets", \
    "--allow-write=/app/secrets", \
    "--allow-env=CONNECTIVITY_CHECK,CONNECTIVITY_TIMEOUT,DENO_ENV,DEPLOYMENT_ENV,HTTP_PROXY,HTTPS_PROXY,HTTP_TIMEOUT,INOREADER_CLIENT_ID,INOREADER_CLIENT_ID_FILE,INOREADER_CLIENT_SECRET,INOREADER_CLIENT_SECRET_FILE,INOREADER_REDIRECT_URI,INTERNAL_AUTH_TOKEN,INTERNAL_AUTH_TOKEN_FILE,LOG_INCLUDE_STACK_TRACE,LOG_INCLUDE_TIMESTAMP,LOG_LEVEL,NETWORK_FALLBACK_TO_DIRECT,NODE_ENV,OTEL_ENABLED,OTEL_EXPORTER_OTLP_ENDPOINT,OTEL_SERVICE_NAME,RETRY_BACKOFF_FACTOR,RETRY_BASE_DELAY,RETRY_MAX_ATTEMPTS,RETRY_MAX_DELAY,SERVICE_VERSION,TOKEN_STORAGE_PATH", \
    "main.ts", "daemon"]