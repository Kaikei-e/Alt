# POST /api/v1/extract-tags with a missing `title` field — FastAPI +
# Pydantic v2 reject the request at schema validation time, returning
# 422 Unprocessable Entity with a `detail` array. The endpoint must
# never reach the ML pipeline for an invalid payload.
#
# Scenarios 02/03 already warmed the background tag service, so the
# validation branch is exercised against a ready server; no retry is
# needed here.
POST {{base_url}}/api/v1/extract-tags
Content-Type: application/json
{
  "content": "body-only payload without title field"
}
HTTP 422
[Asserts]
jsonpath "$.detail" isCollection
jsonpath "$.detail[0].loc" isCollection
jsonpath "$.detail[0].type" exists
