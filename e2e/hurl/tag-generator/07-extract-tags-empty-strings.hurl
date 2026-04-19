# POST /api/v1/extract-tags with empty strings — Pydantic accepts them
# (`str` has no min_length constraint on the model) so the handler
# invokes the extractor. With no input signal the pipeline returns
# success=true with an empty tag list and confidence 0.0; `language` is
# still detected (defaults to English on empty input). This pins the
# graceful no-signal arm of the ML pipeline.
POST {{base_url}}/api/v1/extract-tags
Content-Type: application/json
{
  "title": "",
  "content": ""
}
HTTP 200
[Asserts]
jsonpath "$.success" == true
jsonpath "$.tags" isCollection
jsonpath "$.tags" count == 0
jsonpath "$.confidence" == 0
jsonpath "$.inference_ms" isNumber
jsonpath "$.inference_ms" >= 0
jsonpath "$.language" exists
