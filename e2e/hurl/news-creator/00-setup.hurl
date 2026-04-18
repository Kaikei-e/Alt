# news-creator readiness probe — must run first. Retries /health on
# :11434 until the FastAPI lifespan finishes wiring OllamaGateway and
# the uvicorn worker binds the port. With WARMUP_ENABLED=false this is
# fast (one /api/tags probe against the stub), but Python cold-start
# still takes a couple of seconds on a 2-core CI runner.
GET {{base_url}}/health
[Options]
retry: 30
retry-interval: 500
HTTP 200
[Asserts]
jsonpath "$.status" == "healthy"
jsonpath "$.service" == "news-creator"
