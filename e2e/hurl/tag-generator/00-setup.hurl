# tag-generator readiness probe — must run first. Retries /health on
# :9400 until the container's own healthcheck settles. FastAPI lifespan
# blocks on SBERT warm-up + Redis Streams consumer thread spawn, so
# /health can briefly refuse connections before the port is bound even
# though the CMD has started.
GET {{base_url}}/health
[Options]
retry: 30
retry-interval: 500
HTTP 200
[Asserts]
jsonpath "$.status" == "healthy"
jsonpath "$.service" == "tag-generator"
