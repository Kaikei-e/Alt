# Minimal Ollama-compatible stub used by the news-creator Hurl staging
# slice. Exposes just enough of the Ollama HTTP surface for Phase 1 of the
# news-creator E2E suite (`/api/tags` for OllamaGateway.list_models()).
# Subsequent phases will extend it with `/api/generate`, `/api/chat`, and
# an `/admin/*` control endpoint for queue-saturation scenarios.
FROM python:3.13-slim

RUN pip install --no-cache-dir 'fastapi==0.118.0' 'uvicorn[standard]==0.30.6'

WORKDIR /app
COPY app.py /app/app.py

EXPOSE 11435

HEALTHCHECK --interval=3s --timeout=3s --start-period=2s --retries=10 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:11435/api/tags').read()" || exit 1

CMD ["python", "-m", "uvicorn", "app:app", "--host", "0.0.0.0", "--port", "11435", "--log-level", "info"]
