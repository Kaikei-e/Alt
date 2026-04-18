# /api/generate — Ollama-pass-through endpoint. Validates the response
# is shaped like Ollama's /api/generate (`{model, response, done,
# done_reason, prompt_eval_count, eval_count, total_duration}`) since
# generate_handler.py forwards the LLMGenerateResponse fields through.
POST {{base_url}}/api/generate
Content-Type: application/json
file,e2e/fixtures/news-creator/generate-prompt.json;
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.model" isString
jsonpath "$.response" isString
jsonpath "$.response" matches ".+"
jsonpath "$.done" == true
jsonpath "$.done_reason" == "stop"
jsonpath "$.prompt_eval_count" isInteger
jsonpath "$.eval_count" isInteger
jsonpath "$.total_duration" isInteger
