# /v1/summary/generate — recap-worker structured-output path.
# `_call_llm_for_recap` posts to /api/chat with a `format` JSON schema;
# the stub recognizes the `format` field and returns RecapSummary-shaped
# JSON in `message.content`, which `_parse_summary_json` unwraps into
# the final RecapSummaryResponse.
POST {{base_url}}/v1/summary/generate
Content-Type: application/json
file,e2e/fixtures/news-creator/recap-clusters.json;
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.job_id" == "00000000-0000-4000-8000-000000000101"
jsonpath "$.genre" == "ai"
jsonpath "$.summary.title" isString
jsonpath "$.summary.title" matches ".+"
jsonpath "$.summary.bullets" isCollection
jsonpath "$.summary.bullets" count >= 1
jsonpath "$.summary.language" == "ja"
jsonpath "$.metadata.model" isString
jsonpath "$.metadata.summary_length_bullets" >= 1
jsonpath "$.metadata.is_degraded" == false
