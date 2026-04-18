# recap-pipeline-stub — minimal FastAPI service that masquerades as
# recap-worker's four upstream neighbours during Hurl E2E. Built in CI
# alongside recap-worker; never deployed outside e2e/hurl/recap-worker/.
FROM python:3.13-alpine

WORKDIR /app

RUN pip install --no-cache-dir \
        "fastapi==0.115.6" \
        "uvicorn[standard]==0.32.1"

COPY main.py /app/main.py

EXPOSE 80

# Bind on port 80 so URLs like http://news-creator/api/health resolve
# without an explicit port — recap-worker's clients call into the bare
# scheme://host base URL we hand them via env.
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "80", "--log-level", "info"]
