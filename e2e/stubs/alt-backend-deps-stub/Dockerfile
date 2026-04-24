# alt-backend-deps-stub — minimal FastAPI service that masquerades as
# every upstream alt-backend talks to during the Hurl E2E suite. Built in
# CI alongside alt-backend; never deployed outside e2e/hurl/alt-backend/.
FROM python:3.13-alpine

WORKDIR /app

RUN pip install --no-cache-dir \
        "fastapi==0.115.6" \
        "uvicorn[standard]==0.32.1" \
        "protobuf==5.29.6"
# protobuf runtime MUST be >= the gencode version baked into gen/. The local
# protoc used by ./regenerate.sh currently emits gencode 5.29.2; pinning the
# runtime to the latest 5.29.x patch (5.29.6) gives us headroom without
# crossing a major that protobuf guarantees nothing about. If regenerate.sh
# starts producing gencode newer than this, bump the pin here in lockstep.

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
