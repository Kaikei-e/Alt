# Full CRUD lifecycle for a report created with a scope brief
# (scope.topic + scope.dateRange). Exercises the brief insert path at
# connect_service.py:71-74 — when scope contains a "topic" key, a
# report_briefs row is inserted alongside the report. GetReport then
# echoes the brief back via the report.scope map.
POST {{base_url}}/alt.acolyte.v1.AcolyteService/CreateReport
Content-Type: application/json
Connect-Protocol-Version: 1
file,e2e/fixtures/acolyte-orchestrator/report-create-with-topic.json;
HTTP 200
[Captures]
report_id: jsonpath "$.reportId"
[Asserts]
jsonpath "$.reportId" matches "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"

POST {{base_url}}/alt.acolyte.v1.AcolyteService/GetReport
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "reportId": "{{report_id}}"
}
HTTP 200
[Asserts]
jsonpath "$.report.reportId" == "{{report_id}}"
jsonpath "$.report.title" == "hurl-acolyte-with-topic"
jsonpath "$.report.reportType" == "market_analysis"
jsonpath "$.report.scope.topic" == "AI infrastructure trends"
jsonpath "$.report.scope.dateRange" == "2026-04"

POST {{base_url}}/alt.acolyte.v1.AcolyteService/DeleteReport
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "reportId": "{{report_id}}"
}
HTTP 200
