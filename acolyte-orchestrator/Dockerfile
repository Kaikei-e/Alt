FROM ghcr.io/astral-sh/uv:0.11.0 AS uv

FROM python:3.14-slim AS builder

COPY --from=uv /uv /usr/local/bin/uv

WORKDIR /app

# Install dependencies first (cached layer)
COPY pyproject.toml uv.lock .python-version ./
RUN uv sync --frozen --no-install-project

# Copy application
COPY main.py ./
COPY acolyte/ ./acolyte/

# Install project
RUN uv sync --frozen

FROM python:3.14-slim

RUN apt-get update && apt-get install -y --no-install-recommends curl && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --uid 1000 appuser
USER appuser
WORKDIR /app

COPY --from=builder --chown=appuser:appuser /app /app

EXPOSE 8090

CMD ["/app/.venv/bin/uvicorn", "main:create_app", "--factory", "--host", "0.0.0.0", "--port", "8090"]
