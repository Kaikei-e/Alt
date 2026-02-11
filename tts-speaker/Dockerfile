# ==============================================================================
# ビルドステージ
# ==============================================================================
# ROCm 7.2 + PyTorch 2.9 (Preview) base image
FROM rocm/pytorch:rocm7.2_ubuntu24.04_py3.12_pytorch_release_2.9.1 AS builder

WORKDIR /app

# ビルド依存関係
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
    build-essential \
    gcc \
    cmake \
    python3-dev \
    espeak-ng \
    && rm -rf /var/lib/apt/lists/*

# uv を公式イメージからコピー
COPY --from=ghcr.io/astral-sh/uv:0.5.14 /uv /uvx /bin/

# キャッシュ設定
ENV UV_COMPILE_BYTECODE=0 UV_LINK_MODE=copy

# 【レイヤーキャッシュ最適化】依存関係ファイルのみを先にコピー
COPY pyproject.toml ./

# 仮想環境を作成 (system site-packages を使うか、uv venv で新規作成するか)
# rocm/pytorch イメージの PyTorch を使いたいので、--system-site-packages を使う
RUN uv venv --system-site-packages

# 依存関係をインストール
# Torch はプリインストールされているので除外する設定が必要だが、
# uv pip install で torch を除外するか、pyproject.toml から torch を外すか。
# ここでは単純に uv sync すると torch を pypi から取ってきて上書きしてしまう可能性がある。
# よって、pyproject.toml の torch 依存を無視させるか、
# uv pip install -r <(uv pip compile pyproject.toml | grep -v torch) のような工夫が必要。
# または、以前のように extra-index-url を指定するが、7.2 の wheel が公開されていないなら
# プリインストールされた torch を使う必要がある。

# 一旦、requirements.txt を生成して torch を削除してからインストールする戦略をとる
RUN uv pip compile pyproject.toml -o requirements.txt \
    && sed -i '/torch/d' requirements.txt \
    && sed -i '/kokoro/d' requirements.txt \
    && uv pip install -r requirements.txt

# kokoro (depends on torch) を別途インストール (依存関係チェックをスキップ)
RUN uv pip install --no-deps kokoro>=0.9.4

# アプリケーションコードをコピー
COPY tts_speaker ./tts_speaker

# プロジェクトをインストール
RUN uv pip install --no-deps .

# Download UniDic dictionary
RUN . .venv/bin/activate && python -m unidic download

# ==============================================================================
# ランタイムステージ
# ==============================================================================
FROM rocm/pytorch:rocm7.2_ubuntu24.04_py3.12_pytorch_release_2.9.1

WORKDIR /app

# ランタイム依存関係
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
    HF_HOME=/app/.cache/huggingface \
    HSA_OVERRIDE_GFX_VERSION=11.0.0 \
    PYTORCH_HIP_ALLOC_CONF=expandable_segments:True \
    HIP_VISIBLE_DEVICES=0

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
