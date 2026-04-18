# /v1/summary/generate/batch — bundled multi-genre call. `responses[]`
# carries the successful outputs and `errors[]` is populated on
# per-request failure. With the stub, every request succeeds, so
# `errors` should be an empty collection.
POST {{base_url}}/v1/summary/generate/batch
Content-Type: application/json
file,e2e/fixtures/news-creator/recap-batch.json;
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.responses" isCollection
jsonpath "$.responses" count == 2
jsonpath "$.responses[0].job_id" == "00000000-0000-4000-8000-000000000201"
jsonpath "$.responses[0].genre" == "ai"
jsonpath "$.responses[0].summary.bullets" isCollection
jsonpath "$.responses[0].summary.bullets" count >= 1
jsonpath "$.responses[1].job_id" == "00000000-0000-4000-8000-000000000202"
jsonpath "$.responses[1].genre" == "security"
jsonpath "$.errors" isCollection
jsonpath "$.errors" count == 0
