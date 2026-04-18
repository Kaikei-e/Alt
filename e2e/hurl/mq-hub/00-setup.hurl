# mq-hub readiness probe — must run first. Waits for compose-healthcheck
# settle by retrying /health until the service answers 200 with
# redis_status=="connected".
GET {{base_url}}/health
[Options]
retry: 30
retry-interval: 500
HTTP 200
[Asserts]
jsonpath "$.healthy" == true
jsonpath "$.redis_status" == "connected"
