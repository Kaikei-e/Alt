# POST /api/v1/extract-tags with keyword-dense English content — the
# ML pipeline (KeyBERT over a multilingual MiniLM SBERT) should yield
# at least one tag for this text. The scenario doubles as an SBERT
# warm-up for 04-generate-tags-via-mqhub.hurl (first inference on a
# cold container costs several seconds; subsequent ones are bounded).
POST {{base_url}}/api/v1/extract-tags
Content-Type: application/json
file,e2e/fixtures/tag-generator/extract-tags-keyword.json;
HTTP 200
[Asserts]
jsonpath "$.success" == true
jsonpath "$.tags" isCollection
jsonpath "$.tags" count > 0
jsonpath "$.confidence" > 0.0
jsonpath "$.inference_ms" > 0
jsonpath "$.language" exists
