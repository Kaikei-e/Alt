# ==============================================================================
# recap-evaluator Dockerfile
# マルチステージビルド + 非rootユーザー
# ==============================================================================

# ビルドステージ
FROM python:3.13-slim AS builder

WORKDIR /app

# uv を公式イメージからコピー (バージョン固定)
COPY --from=ghcr.io/astral-sh/uv:0.5.14 /uv /uvx /bin/

ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy

# 依存関係ファイルをコピー
COPY pyproject.toml ./
COPY uv.lock* ./

# 依存関係を同期 (仮想環境作成)
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-install-project

# ランタイムステージ
FROM python:3.13-slim

WORKDIR /app

# ヘルスチェック用 curl
RUN apt-get update && apt-get install -y --no-install-recommends curl \
    && rm -rf /var/lib/apt/lists/*

# 非rootユーザー作成
RUN groupadd -r evaluator && useradd -r -g evaluator evaluator

# 仮想環境をコピー
COPY --from=builder /app/.venv /app/.venv

# 環境変数
ENV PATH="/app/.venv/bin:$PATH" \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONPATH="/app/src"

# アプリケーションコードをコピー
COPY --chown=evaluator:evaluator src/ ./src/

USER evaluator

EXPOSE 8080

# ヘルスチェック
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

CMD ["python", "-m", "uvicorn", "recap_evaluator.main:app", "--host", "0.0.0.0", "--port", "8080"]
