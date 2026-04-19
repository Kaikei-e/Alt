# POST /api/v1/extract-tags with a missing `content` field — the
# Pydantic model requires both title and content, so the missing field
# yields 422 at schema validation before the ML pipeline runs.
POST {{base_url}}/api/v1/extract-tags
Content-Type: application/json
{
  "title": "title-only payload"
}
HTTP 422
[Asserts]
jsonpath "$.detail" isCollection
jsonpath "$.detail[0].loc" isCollection
