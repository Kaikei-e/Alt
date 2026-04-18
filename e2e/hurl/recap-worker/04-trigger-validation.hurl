# normalize_genres (api/generate.rs:87-100) trims and dedupes; if every
# entry is empty after trim the handler returns 400 with the error
# message below (api/generate.rs:53-58). This pins the validation
# contract so accidentally accepting an all-empty array would surface.
POST {{base_url}}/v1/generate/recaps/7days
Content-Type: application/json
file,e2e/fixtures/recap-worker/trigger-7days-invalid.json;
HTTP 400
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.error" == "genres array must include at least one non-empty value"
