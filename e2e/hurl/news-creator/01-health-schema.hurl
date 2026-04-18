# /health contract — pinned to handler/health_handler.py. FastAPI
# returns snake_case JSON via the default encoder; no proto3-JSON
# camelCase here. The readiness probe in 00-setup.hurl already absorbed
# lifespan startup; this scenario locks the schema and verifies the
# stub Ollama exposed at least one model name through list_models().
GET {{base_url}}/health
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.status" == "healthy"
jsonpath "$.service" == "news-creator"
jsonpath "$.models" isCollection
jsonpath "$.models" count >= 1
jsonpath "$.models[0].name" == "gemma3:4b-it-qat"
# Health handler attaches `error` only when list_models() raises. With
# the stub responding 200 to /api/tags, the field must be absent.
jsonpath "$.error" not exists
