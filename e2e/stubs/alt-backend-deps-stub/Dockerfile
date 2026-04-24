# alt-backend-deps-stub — minimal FastAPI service that masquerades as
# every upstream alt-backend talks to during the Hurl E2E suite. Built in
# CI alongside alt-backend; never deployed outside e2e/hurl/alt-backend/.
FROM python:3.13-alpine

WORKDIR /app

RUN pip install --no-cache-dir \
        "fastapi==0.115.6" \
        "uvicorn[standard]==0.32.1" \
        "protobuf==5.29.1"

# Pre-generated Python proto bindings for the knowledge-sovereign RPCs the
# stub answers as proto binary (alt-backend's sovereign_client negotiates
# application/proto by default). Regenerate via ./regenerate.sh when
# proto/services/sovereign/v1/sovereign.proto changes.
COPY gen /app/gen
ENV PYTHONPATH=/app/gen

COPY main.py /app/main.py

EXPOSE 80

# Bind on port 80 so upstream URLs like http://search-indexer/... resolve
# without an explicit port — alt-backend's clients call into the bare
# scheme://host base URL we hand them via env.
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "80", "--log-level", "info"]
