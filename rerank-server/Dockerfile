# syntax=docker/dockerfile:1.7
# ==============================================================================
# rerank-server Dockerfile
# CPU-only, dynamic int8-quantized (avx2) ONNX reranker. Replaces the remote
# Mac reranker for local deployments (RERANK_BACKEND=onnx). Model weights are
# NOT baked into the image: the entrypoint downloads BAAI/bge-reranker-v2-m3
# (~2.3GB) and quantizes it into the /models volume on first boot.
# 2-stage build (builder -> runtime) with uv, CPU-only PyTorch, non-root user.
# ==============================================================================

FROM python:3.14-slim@sha256:b877e50bd90de10af8d82c57a022fc2e0dc731c5320d762a27986facfc3355c1 AS builder

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        build-essential \
    && rm -rf /var/lib/apt/lists/*

# Pinned by digest per security-auditor F-001.
COPY --from=ghcr.io/astral-sh/uv:0.11.0@sha256:e49fde5daf002023f0a2e2643861ce9ca8a8da5b73d0e6db83ef82ff99969baf /uv /uvx /bin/

ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_PYTHON_DOWNLOADS=never

COPY pyproject.toml uv.lock ./

RUN uv venv

# Dep-only layer: stays cached while rerank_server.py churns. The "onnx"
# group (onnxruntime + optimum[onnxruntime]) is CPU-only and separate from
# the Mac/MPS deploy path (requirements.txt), which never installs it.
RUN --mount=type=cache,id=uv-rerank-server,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-install-project --no-editable --group onnx

COPY rerank_server.py ./

RUN --mount=type=cache,id=uv-rerank-server,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-editable --group onnx

# Post-install slim: drop vendored tests, torch headers, and .pyi stubs.
RUN set -eux; \
    SITE="/app/.venv/lib/python3.14/site-packages"; \
    test -d "$SITE"; \
    for pkg in numpy scipy sklearn transformers torch sentence_transformers \
               onnxruntime optimum tokenizers huggingface_hub; do \
        if [ -d "$SITE/$pkg" ]; then \
            find "$SITE/$pkg" -type d \( -name tests -o -name test \) -prune -exec rm -rf {} +; \
        fi; \
    done; \
    rm -rf "$SITE/torch/include"; \
    find "$SITE" -name "*.pyi" -delete; \
    find "$SITE" -name "__pycache__" -type d -prune -exec rm -rf {} +; \
    find "$SITE" -name "*.pyc" -delete

# Verification (stays in builder, does not bloat runtime).
RUN /app/.venv/bin/python -c "\
import torch, sentence_transformers, onnxruntime, optimum.onnxruntime; \
print(f'torch {torch.__version__}'); \
print(f'sentence-transformers {sentence_transformers.__version__}'); \
print(f'onnxruntime {onnxruntime.__version__}'); \
assert not torch.cuda.is_available(), 'torch CUDA must be disabled'"

# ==============================================================================
# Runtime stage
# ==============================================================================
FROM python:3.14-slim@sha256:b877e50bd90de10af8d82c57a022fc2e0dc731c5320d762a27986facfc3355c1 AS runtime

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && apt-get clean && rm -rf /var/lib/apt/lists/* \
    && groupadd -g 1000 appuser \
    && useradd -u 1000 -g 1000 -m -s /bin/bash appuser \
    && mkdir -p /models \
    && chown -R appuser:appuser /models

ENV PATH="/app/.venv/bin:$PATH" \
    VIRTUAL_ENV=/app/.venv \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    RERANK_BACKEND=onnx \
    RERANK_MODEL_DIR=/models/bge-reranker-v2-m3-onnx \
    OMP_NUM_THREADS=16

COPY --from=builder --chown=appuser:appuser /app/.venv /app/.venv
COPY --chown=appuser:appuser rerank_server.py ./

USER appuser

EXPOSE 8080

# security-auditor F-003 pattern: stdlib probe instead of curl to keep the
# runtime image minimal. urlopen() raises (non-zero exit) on the 503 that
# /health returns while the model is still loading/exporting.
HEALTHCHECK --interval=30s --timeout=5s --start-period=900s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8080/health', timeout=3)" || exit 1

CMD ["python", "-m", "uvicorn", "rerank_server:app", "--host", "0.0.0.0", "--port", "8080"]
