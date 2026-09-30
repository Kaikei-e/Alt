# syntax=docker/dockerfile:1.7
FROM python:3.10-slim@sha256:9d53d8d4c0e882f61913025db53b3aec4ef74336082a9ab47d8a14e9e8329b00 AS builder

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    DEBIAN_FRONTEND=noninteractive \
    UV_LINK_MODE=copy \
    UV_PROJECT_ENVIRONMENT=/app/.venv

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        git \
    && rm -rf /var/lib/apt/lists/*

COPY --from=ghcr.io/astral-sh/uv:0.11.0@sha256:e49fde5daf002023f0a2e2643861ce9ca8a8da5b73d0e6db83ef82ff99969baf /uv /uvx /bin/

ADD https://github.com/Aratako/Irodori-TTS-Server.git#61012c760f22f7b4a6c21c5c5f8f9e148120b6f9 /app
ADD https://github.com/Aratako/Irodori-TTS.git#89f9d8fbd4d51ea019867ee1197725ede1df13c5 /opt/irodori-tts

RUN --mount=type=cache,target=/root/.cache/uv,sharing=locked \
    uv sync --locked --no-dev --no-editable --extra cu128

FROM python:3.10-slim@sha256:9d53d8d4c0e882f61913025db53b3aec4ef74336082a9ab47d8a14e9e8329b00 AS runtime

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    DEBIAN_FRONTEND=noninteractive \
    PATH="/app/.venv/bin:$PATH" \
    PYTHONPATH=/app \
    HOME=/home/appuser

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        ffmpeg \
        libsndfile1 \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd -g 1000 appuser \
    && useradd -u 1000 -g appuser -m -s /bin/bash appuser \
    && mkdir -p /home/appuser/.cache \
    && chown -R appuser:appuser /home/appuser /app

COPY --chown=1000:1000 --from=builder /app/.venv /app/.venv
COPY --chown=1000:1000 --from=builder /opt/irodori-tts /opt/irodori-tts
COPY --chown=1000:1000 alt_irodori /app/alt_irodori

USER appuser

EXPOSE 8088

CMD ["python", "-m", "alt_irodori"]
