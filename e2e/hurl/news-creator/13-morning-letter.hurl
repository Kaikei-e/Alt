# /v1/morning-letter/generate — daily briefing generator. The usecase
# posts to /api/generate with `format=MorningLetterContent.model_json_schema()`;
# the stub recognizes the schema by spotting `sections` + `lead` in
# `format.properties` and returns a `MorningLetterContent`-shaped JSON
# string that `_parse_content` unwraps into the response.
POST {{base_url}}/v1/morning-letter/generate
Content-Type: application/json
file,e2e/fixtures/news-creator/morning-letter-request.json;
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.target_date" == "2026-04-18"
jsonpath "$.edition_timezone" == "Asia/Tokyo"
jsonpath "$.content.schema_version" == 1
jsonpath "$.content.lead" isString
jsonpath "$.content.lead" matches ".+"
jsonpath "$.content.sections" isCollection
jsonpath "$.content.sections" count >= 1
jsonpath "$.content.sections[0].key" matches "^(lead|top3|what_changed|by_genre:.+)$"
jsonpath "$.content.sections[0].title" isString
jsonpath "$.content.sections[0].bullets" isCollection
jsonpath "$.content.sections[0].bullets" count >= 1
jsonpath "$.content.generated_at" isString
jsonpath "$.metadata.model" isString
jsonpath "$.metadata.summary_length_bullets" >= 1
