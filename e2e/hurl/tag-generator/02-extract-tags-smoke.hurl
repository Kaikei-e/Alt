# POST /api/v1/extract-tags minimal payload — proves the FastAPI
# lifespan finished initializing `_background_tag_service` (which is
# started in a background thread spawned from the lifespan). If the
# service is still warming up, the endpoint returns 503 with
# "Tag extraction service not ready" (auth_service.py:519). The retry
# here absorbs that race: the healthcheck that gates 00-setup.hurl only
# probes /health, which does not wait on the background thread. On
# GitHub-hosted 2-core runners the SBERT forward pass inside
# TagExtractor.warmup() has been observed to run past 10 s from cold
# container start, so the budget here is 60 s with a 1 s interval.
#
# Response shape is snake_case (Pydantic + FastAPI default encoder):
# {"success": true, "tags": [...], "confidence": 0.0, "inference_ms": 0.0, "language": "en"}
POST {{base_url}}/api/v1/extract-tags
Content-Type: application/json
[Options]
retry: 180
retry-interval: 1000
file,e2e/fixtures/tag-generator/extract-tags-smoke.json;
HTTP 200
[Asserts]
jsonpath "$.success" == true
jsonpath "$.tags" isCollection
jsonpath "$.confidence" isNumber
jsonpath "$.inference_ms" isNumber
jsonpath "$.inference_ms" >= 0
jsonpath "$.language" exists
