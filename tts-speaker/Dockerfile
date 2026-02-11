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
COPY --from=ghcr.io/astral-sh/uv:0.5.14 /uv /uvx /bin/

# キャッシュ設定
ENV UV_COMPILE_BYTECODE=0 UV_LINK_MODE=copy VIRTUAL_ENV=/opt/venv

# 【レイヤーキャッシュ最適化】依存関係ファイルのみを先にコピー
COPY pyproject.toml ./

# 依存関係をインストール
# uv pip compile で全依存関係を解決し、GPU エコシステム (torch, nvidia, cuda, triton) を除外
# ベースイメージに ROCm 版 torch がプリインストール済みのため、PyPI の CUDA 版との競合を回避
# --no-deps で依存解決をスキップ (compile 済みで全パッケージ列挙されているため安全)
RUN uv pip compile pyproject.toml -o requirements.txt \
    && sed -i -E '/(^torch|^kokoro|^nvidia-|^cuda-|^triton)/d' requirements.txt \
    && echo "=== Installing (torch/CUDA excluded) ===" \
    && uv pip install --no-deps -r requirements.txt

# kokoro をインストール (torch はベースイメージから利用)
RUN uv pip install --no-deps "kokoro>=0.9.4"

# アプリケーションコードをコピー
COPY tts_speaker ./tts_speaker

# プロジェクトをインストール
RUN uv pip install --no-deps .

# Download UniDic dictionary
RUN python3 -m unidic download

# キャッシュディレクトリ作成 & 権限設定
RUN groupadd -r tts && useradd -r -g tts tts \
    && mkdir -p /app/.cache/huggingface \
    && chown -R tts:tts /app/.cache \
    && chown -R tts:tts /app/tts_speaker

# 環境変数
ENV HOME=/app \
    OMP_NUM_THREADS=2 \
    HF_HOME=/app/.cache/huggingface \
    HSA_OVERRIDE_GFX_VERSION=11.0.0 \
    PYTORCH_HIP_ALLOC_CONF=expandable_segments:True \
    HIP_VISIBLE_DEVICES=0

USER tts

EXPOSE 9700

CMD ["python3", "-m", "uvicorn", "tts_speaker.app.main:create_app", "--factory", "--host", "0.0.0.0", "--port", "9700"]
