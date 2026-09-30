# syntax=docker/dockerfile:1.7
FROM python:3.14-slim@sha256:b877e50bd90de10af8d82c57a022fc2e0dc731c5320d762a27986facfc3355c1 AS builder

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY --from=ghcr.io/astral-sh/uv:0.11.0 /uv /uvx /bin/

ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy

COPY pyproject.toml uv.lock ./

RUN uv venv

RUN --mount=type=cache,id=uv-tts-speaker,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-install-project --no-editable

COPY tts_speaker ./tts_speaker

RUN --mount=type=cache,id=uv-tts-speaker,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-editable

RUN set -eux; \
    SITE="/app/.venv/lib/python3.14/site-packages"; \
    test -d "$SITE"; \
    find "$SITE" -name "*.pyi" -delete; \
    find "$SITE" -name "__pycache__" -type d -prune -exec rm -rf {} +; \
    find "$SITE" -name "*.pyc" -delete

FROM python:3.14-slim@sha256:b877e50bd90de10af8d82c57a022fc2e0dc731c5320d762a27986facfc3355c1 AS runtime

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
    LOG_LEVEL=INFO

COPY --from=builder --chown=appuser:appuser /app/.venv /app/.venv
COPY --chown=appuser:appuser tts_speaker ./tts_speaker

EXPOSE 9700 9443

USER appuser

CMD ["python", "-m", "tts_speaker"]
