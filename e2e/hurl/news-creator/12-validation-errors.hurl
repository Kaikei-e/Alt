# Validation error paths. summarize_handler enforces a 100-char
# minimum on `content` and rejects with HTTP 400 (handler/summarize_handler.py
# lines 67-83). generate_handler enforces a 240_000-char prompt cap
# and rejects with HTTP 400 (handler/generate_handler.py lines 60-74).
# A truly malformed JSON body produces FastAPI's default 422.
#
# All three checks are stateless reads and do not enter the
# HybridPrioritySemaphore queue, so they're safe to run serially
# alongside the other scenarios.

# 1. Content shorter than 100 chars → 400
POST {{base_url}}/api/v1/summarize
Content-Type: application/json
file,e2e/fixtures/news-creator/article-too-short.json;
HTTP 400
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.detail" matches "(?i)content is too short"

# 2. /api/generate with empty prompt → 400 (Pydantic min_length=1).
POST {{base_url}}/api/generate
Content-Type: application/json
{
  "prompt": "",
  "model": "gemma3:4b-it-qat",
  "stream": false
}
HTTP 422
[Asserts]
header "Content-Type" contains "application/json"

# 3. Malformed JSON body on /api/v1/summarize → FastAPI 422.
# Loaded via fixture file because Hurl literal bodies must be valid
# JSON when Content-Type says so.
POST {{base_url}}/api/v1/summarize
Content-Type: application/json
file,e2e/fixtures/news-creator/malformed-summarize.json;
HTTP 422
[Asserts]
header "Content-Type" contains "application/json"
