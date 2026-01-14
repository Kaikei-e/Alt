# Dockerfile.app - FastAPI application only
# Separated from Ollama for faster deployments and smaller images

FROM python:3.11-slim

# Install system dependencies
RUN apt-get update \
    && apt-get install --no-install-recommends -y \
    ca-certificates \
    curl \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Install uv for Python package management
COPY --from=ghcr.io/astral-sh/uv:0.5.14 /uv /bin/uv

# Create application user
RUN groupadd -g 1000 appuser \
    && useradd -u 1000 -g 1000 -m -s /bin/bash appuser

WORKDIR /app

# Install Python application dependencies first (for better caching)
COPY --chown=appuser:appuser app/pyproject.toml app/uv.lock /app/
RUN UV_SYSTEM_PYTHON=1 uv pip install --break-system-packages --no-cache -r /app/pyproject.toml

# Copy application code
COPY --chown=appuser:appuser app/ /app/

# Environment configuration
ENV LLM_SERVICE_URL=http://news-creator-backend:11435
ENV LOG_LEVEL=INFO

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:11434/health || exit 1

EXPOSE 11434

# Runtime user
USER appuser

# Start FastAPI application
CMD ["python", "-m", "uvicorn", "main:app", "--host", "0.0.0.0", "--port", "11434", "--log-level", "info"]
