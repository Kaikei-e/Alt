# ==============================================================================
# ビルドステージ
# ==============================================================================
FROM python:3.12-slim AS builder

WORKDIR /app

# ビルド依存関係 (espeak-ng は misaki[ja] の phonemizer が必要)
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
    build-essential \
    gcc \
    espeak-ng \
    && rm -rf /var/lib/apt/lists/*

# uv を公式イメージからコピー
COPY --from=ghcr.io/astral-sh/uv:0.5.14 /uv /uvx /bin/

# キャッシュ設定
ENV UV_COMPILE_BYTECODE=0 UV_LINK_MODE=copy

# 【レイヤーキャッシュ最適化】依存関係ファイルのみを先にコピー
COPY pyproject.toml uv.lock ./

# 仮想環境を作成
RUN uv venv

# PyTorch CPU版をインストール（82Mパラメータは CPU で実用速度）
RUN --mount=type=cache,id=uv-tts-speaker,target=/root/.cache/uv \
    uv pip install torch --index-url https://download.pytorch.org/whl/cpu

# 依存関係をインストール（プロジェクト自体は後でインストール）
RUN --mount=type=cache,id=uv-tts-speaker,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-install-project

# アプリケーションコードをコピー
COPY tts_speaker ./tts_speaker

# プロジェクトをインストール
RUN --mount=type=cache,id=uv-tts-speaker,target=/root/.cache/uv \
    uv sync --frozen --no-dev

# Download UniDic dictionary
RUN . .venv/bin/activate && python -m unidic download

# ==============================================================================
# ランタイムステージ
# ==============================================================================
FROM python:3.12-slim

WORKDIR /app

# ランタイム依存関係 (espeak-ng + curl for healthcheck)
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
    espeak-ng \
    curl \
    && rm -rf /var/lib/apt/lists/*

# 非rootユーザー作成
RUN groupadd -r tts && useradd -r -g tts tts

# 環境変数
ENV PATH="/app/.venv/bin:$PATH" \
    HOME=/app \
    OMP_NUM_THREADS=2 \
    HF_HOME=/app/.cache/huggingface

# 仮想環境をコピー
COPY --from=builder /app/.venv /app/.venv

# アプリケーションコードをコピー
COPY --chown=tts:tts pyproject.toml ./
COPY --chown=tts:tts tts_speaker ./tts_speaker

# キャッシュディレクトリ作成 (HuggingFace モデルキャッシュ用)
RUN mkdir -p /app/.cache/huggingface && chown -R tts:tts /app/.cache

USER tts

EXPOSE 9700

CMD ["python", "-m", "uvicorn", "tts_speaker.app.main:create_app", "--factory", "--host", "0.0.0.0", "--port", "9700"]
