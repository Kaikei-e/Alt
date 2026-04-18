# /api/v1/plan-query — Augur's structured retrieval planner.
# plan_query_usecase posts to /api/chat without a `format` field (Ollama
# #10929 — thinking + format = empty content). The stub returns a JSON
# string matching the QueryPlan schema, which `_extract_json` +
# `QueryPlan(**parsed)` unwrap into the response `plan`.
POST {{base_url}}/api/v1/plan-query
Content-Type: application/json
file,e2e/fixtures/news-creator/plan-query-request.json;
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.original_query" == "OpenAI の最新発表について教えて"
jsonpath "$.model" isString
jsonpath "$.plan.reasoning" isString
jsonpath "$.plan.reasoning" matches ".+"
jsonpath "$.plan.resolved_query" isString
jsonpath "$.plan.search_queries" isCollection
jsonpath "$.plan.search_queries" count >= 1
jsonpath "$.plan.intent" isString
jsonpath "$.plan.retrieval_policy" isString
jsonpath "$.plan.answer_format" isString
jsonpath "$.plan.should_clarify" == false
jsonpath "$.plan.topic_entities" isCollection
