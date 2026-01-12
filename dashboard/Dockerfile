# ==============================================================================
# ビルドステージ
# ==============================================================================
FROM python:3.12-slim AS builder

WORKDIR /app

# ビルド依存関係
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# uv を公式イメージからコピー
COPY --from=ghcr.io/astral-sh/uv:0.5.14 /uv /uvx /bin/

ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy

COPY pyproject.toml uv.lock ./

# 依存関係をインストール (仮想環境作成)
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev

# ==============================================================================
# ランタイムステージ
# ==============================================================================
FROM python:3.12-slim

WORKDIR /app

# ランタイム依存関係のみ (curl はヘルスチェック用)
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    procps \
    && rm -rf /var/lib/apt/lists/*

# 非rootユーザー作成
RUN groupadd -r dashboard && useradd -r -g dashboard dashboard

# 仮想環境をコピー
COPY --from=builder /app/.venv /app/.venv
ENV PATH="/app/.venv/bin:$PATH"

# アプリケーションコードをコピー
COPY --chown=dashboard:dashboard . .

# エントリーポイントスクリプトの権限設定
RUN chmod +x /app/entrypoint.sh

# Streamlit と SSE ポートを公開
EXPOSE 8501 8000

# ヘルスチェック
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
    CMD curl --fail http://localhost:8501/_stcore/health && curl --fail http://localhost:8000/health || exit 1

USER dashboard

ENTRYPOINT ["/app/entrypoint.sh"]
