# /health contract — FastAPI returns snake_case JSON via the default
# encoder (auth_service.py:431-434), no proto3-JSON camelCase involved.
# The readiness probe in 00-setup.hurl already handled retry; this
# scenario just pins the schema.
GET {{base_url}}/health
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.status" == "healthy"
jsonpath "$.service" == "tag-generator"
