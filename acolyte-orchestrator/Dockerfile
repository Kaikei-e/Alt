FROM ghcr.io/astral-sh/uv:0.13.9 AS uv

FROM python:3.15-slim@sha256:65a65fc7b30b06cb6df4930fd4650697cb23f4c2698ab81fc87e4b031bc36928 AS builder

COPY --from=uv /uv /usr/local/bin/uv

WORKDIR /app

# Install dependencies first (cached layer)
COPY pyproject.toml uv.lock .python-version ./
RUN uv sync --frozen --no-dev --no-install-project --no-editable

# Copy application
COPY main.py ./
COPY acolyte/ ./acolyte/

# Install project
RUN uv sync --frozen --no-dev --no-editable

FROM python:3.15-slim@sha256:65a65fc7b30b06cb6df4930fd4650697cb23f4c2698ab81fc87e4b031bc36928

RUN apt-get update && apt-get install -y --no-install-recommends curl && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --uid 1000 appuser
USER appuser
WORKDIR /app

COPY --from=builder --chown=appuser:appuser /app /app

EXPOSE 8090

CMD ["/app/.venv/bin/uvicorn", "main:create_app", "--factory", "--host", "0.0.0.0", "--port", "8090"]
