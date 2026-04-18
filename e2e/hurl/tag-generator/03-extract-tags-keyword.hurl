# POST /api/v1/extract-tags with keyword-dense English content — the
# ML pipeline (KeyBERT over a multilingual MiniLM SBERT) should yield
# at least one tag for this text. The scenario doubles as an SBERT
# warm-up for 04-generate-tags-via-mqhub.hurl (first inference on a
# cold container costs several seconds; subsequent ones are bounded).
# A short retry covers the narrow window where scenario 02 has just
# passed on its final retry: `_background_tag_service` is non-None but
# the KeyBERT pipeline may still be threading its first real input.
POST {{base_url}}/api/v1/extract-tags
Content-Type: application/json
[Options]
retry: 10
retry-interval: 1000
file,e2e/fixtures/tag-generator/extract-tags-keyword.json;
HTTP 200
[Asserts]
jsonpath "$.success" == true
jsonpath "$.tags" isCollection
jsonpath "$.tags" count > 0
jsonpath "$.confidence" > 0.0
jsonpath "$.inference_ms" > 0
jsonpath "$.language" exists
