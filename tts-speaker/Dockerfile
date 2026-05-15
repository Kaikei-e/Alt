# ==============================================================================
# ROCm 7.2 + PyTorch 2.9 (Preview) base image
# ==============================================================================
FROM rocm/pytorch:rocm7.2_ubuntu24.04_py3.12_pytorch_release_2.9.1

WORKDIR /app

# ランタイム依存関係
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
    build-essential \
    gcc \
    cmake \
    python3-dev \
    espeak-ng \
    curl \
    && rm -rf /var/lib/apt/lists/*

# uv を公式イメージからコピー
COPY --from=ghcr.io/astral-sh/uv:0.11.0 /uv /uvx /bin/

# キャッシュ設定
ENV UV_COMPILE_BYTECODE=0 UV_LINK_MODE=copy VIRTUAL_ENV=/opt/venv

# 【レイヤーキャッシュ最適化】依存関係ファイルのみを先にコピー
COPY pyproject.toml ./

# 依存関係をインストール
# uv pip compile で全依存関係を解決し、GPU エコシステム (torch, nvidia, cuda, triton, flash-attn) を除外
# ベースイメージに ROCm 版 torch がプリインストール済みのため、PyPI の CUDA 版との競合を回避
# flash-attn は CUDA 専用、ROCm 上では絶対にインストールしない (qwen-tts は sdpa 経由で動作可)
# --no-deps で依存解決をスキップ (compile 済みで全パッケージ列挙されているため安全)
RUN uv pip compile pyproject.toml -o requirements.txt \
    && sed -i -E '/(^torch|^qwen-tts|^nvidia-|^cuda-|^triton|^flash[-_]attn)/d' requirements.txt \
    && echo "=== Installing (torch/CUDA/flash-attn excluded) ===" \
    && uv pip install --no-deps -r requirements.txt

# qwen-tts をインストール (torch はベースイメージから利用、flash-attn は引き込まない)
RUN uv pip install --no-deps "qwen-tts>=0.1.0" \
    && uv pip uninstall flash-attn 2>/dev/null || true

# アプリケーションコードをコピー
COPY tts_speaker ./tts_speaker

# プロジェクトをインストール
RUN uv pip install --no-deps .

# キャッシュディレクトリ作成 & 権限設定
# MIOpen の lock file / user db / kernel cache は HOME=/app の下に書こうとするが
# tts user では書けない。明示的に /app/.cache/miopen を tts 所有で用意し、env で
# 上書きする (下記 ENV ブロック参照)。
RUN groupadd -r tts && useradd -r -g tts tts \
    && mkdir -p /app/.cache/huggingface /app/.cache/miopen \
    && chown -R tts:tts /app/.cache \
    && chown -R tts:tts /app/tts_speaker

# 環境変数
ENV HOME=/app \
    OMP_NUM_THREADS=2 \
    HF_HOME=/app/.cache/huggingface \
    MIOPEN_USER_DB_PATH=/app/.cache/miopen \
    MIOPEN_CUSTOM_CACHE_DIR=/app/.cache/miopen \
    HSA_OVERRIDE_GFX_VERSION=11.0.0 \
    PYTORCH_HIP_ALLOC_CONF=expandable_segments:True \
    HIP_VISIBLE_DEVICES=0

USER tts

EXPOSE 9700

CMD ["python3", "-m", "uvicorn", "tts_speaker.app.main:create_app", "--factory", "--host", "0.0.0.0", "--port", "9700"]
