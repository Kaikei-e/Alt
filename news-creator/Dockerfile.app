# syntax=docker/dockerfile:1.7
FROM python:3.14-slim AS builder

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY --from=ghcr.io/astral-sh/uv:0.11.0 /uv /uvx /bin/

ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy

COPY app/pyproject.toml app/uv.lock ./

RUN uv venv

RUN --mount=type=cache,id=uv-news-creator,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-install-project --no-editable

COPY app/main.py ./
COPY app/news_creator ./news_creator
COPY app/prompts ./prompts

RUN --mount=type=cache,id=uv-news-creator,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-editable

RUN set -eux; \
    SITE="/app/.venv/lib/python3.14/site-packages"; \
    for pkg in numpy scipy sklearn sentence_transformers transformers torch; do \
        find "$SITE/$pkg" -type d -name tests -prune -exec rm -rf {} + 2>/dev/null || true; \
        find "$SITE/$pkg" -type d -name test  -prune -exec rm -rf {} + 2>/dev/null || true; \
    done

FROM python:3.14-slim AS runtime

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -g 1000 appuser \
    && useradd  -u 1000 -g 1000 -m -s /bin/bash appuser

ENV PATH="/app/.venv/bin:$PATH" \
    VIRTUAL_ENV=/app/.venv \
    LLM_SERVICE_URL=http://news-creator-backend:11435 \
    LOG_LEVEL=INFO \
    OMP_NUM_THREADS=1 \
    MKL_NUM_THREADS=1

COPY --from=builder --chown=appuser:appuser /app/.venv /app/.venv
COPY --chown=appuser:appuser app/main.py ./
COPY --chown=appuser:appuser app/news_creator ./news_creator
COPY --chown=appuser:appuser app/prompts ./prompts

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:11434/health || exit 1

EXPOSE 11434

USER appuser

CMD ["python", "-m", "uvicorn", "main:app", "--host", "0.0.0.0", "--port", "11434", "--log-level", "info"]
