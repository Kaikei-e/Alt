# /api/v1/summarize (non-streaming) — pre-processor / acolyte path.
# Body fixture supplies a >= 100-char Japanese article so the
# `min_content_length=100` guard in summarize_handler.py passes; the
# stub Ollama answers /api/generate with a fixed plain-text response,
# which summarize_usecase rolls into the SummarizeResponse `summary`.
POST {{base_url}}/api/v1/summarize
Content-Type: application/json
file,e2e/fixtures/news-creator/article-normal.json;
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.success" == true
jsonpath "$.article_id" == "hurl-e2e-article-normal"
jsonpath "$.summary" isString
jsonpath "$.summary" matches ".+"
jsonpath "$.model" isString
jsonpath "$.model" matches ".+"
