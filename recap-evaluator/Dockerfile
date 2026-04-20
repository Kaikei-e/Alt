# syntax=docker/dockerfile:1.7
# ==============================================================================
# recap-evaluator Dockerfile
# 2-stage build (builder -> runtime) with uv, CPU-only PyTorch, non-root user.
# Target image size: < 2500 MiB (see .github/workflows/docker-build.yaml gate).
# ==============================================================================

FROM python:3.14-slim AS builder

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY --from=ghcr.io/astral-sh/uv:0.11.0 /uv /uvx /bin/

ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_PYTHON_DOWNLOADS=never

COPY pyproject.toml uv.lock ./

RUN uv venv

RUN --mount=type=cache,id=uv-recap-evaluator,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-install-project

COPY src/ ./src/

RUN --mount=type=cache,id=uv-recap-evaluator,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-editable

# Post-install slim: drop vendored tests, torch headers, and .pyi stubs.
# Keep __pycache__ (UV_COMPILE_BYTECODE output) and *.dist-info (importlib.metadata).
RUN set -eux; \
    SITE="/app/.venv/lib/python3.14/site-packages"; \
    for pkg in numpy scipy sklearn transformers torch bert_score rouge_score pandas; do \
        find "$SITE/$pkg" -type d \( -name tests -o -name test \) -prune -exec rm -rf {} + 2>/dev/null || true; \
    done; \
    rm -rf "$SITE/torch/include" 2>/dev/null || true; \
    find "$SITE" -name "*.pyi" -delete 2>/dev/null || true

FROM python:3.14-slim AS runtime

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r evaluator \
    && useradd -r -g evaluator -m -s /bin/bash evaluator \
    && mkdir -p /app/.cache/huggingface \
    && chown -R evaluator:evaluator /app/.cache

COPY --from=builder --chown=evaluator:evaluator /app/.venv /app/.venv
COPY --chown=evaluator:evaluator src/ ./src/

ENV PATH="/app/.venv/bin:$PATH" \
    VIRTUAL_ENV=/app/.venv \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONPATH="/app/src" \
    HF_HOME="/app/.cache/huggingface" \
    TRANSFORMERS_CACHE="/app/.cache/huggingface" \
    OMP_NUM_THREADS=1 \
    MKL_NUM_THREADS=1

USER evaluator

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

CMD ["python", "-m", "uvicorn", "recap_evaluator.main:app", "--host", "0.0.0.0", "--port", "8080"]
