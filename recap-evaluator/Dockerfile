# recap-evaluator Dockerfile
# Multi-stage build for Python 3.13 with uv

# Stage 1: Build dependencies
FROM python:3.13-slim AS builder

WORKDIR /app

# Install uv
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

# Copy dependency files
COPY pyproject.toml ./
COPY uv.lock* ./

# Sync dependencies (create virtual environment)
RUN uv sync --frozen --no-dev --no-install-project

# Stage 2: Runtime
FROM python:3.13-slim

WORKDIR /app

# Install curl for healthcheck
RUN apt-get update && apt-get install -y --no-install-recommends curl \
    && rm -rf /var/lib/apt/lists/*

# Copy virtual environment from builder
COPY --from=builder /app/.venv /app/.venv
ENV PATH="/app/.venv/bin:$PATH"
ENV PYTHONUNBUFFERED=1
ENV PYTHONDONTWRITEBYTECODE=1

# Copy application code
COPY src/ ./src/

# Set Python path
ENV PYTHONPATH="/app/src"

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Run the application
CMD ["python", "-m", "uvicorn", "recap_evaluator.main:app", "--host", "0.0.0.0", "--port", "8080"]
