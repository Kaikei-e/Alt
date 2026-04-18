# Full CRUD lifecycle for a report created without a scope brief.
# Hurl captures are file-scoped, so the create / read / delete chain
# must live in a single .hurl file. Asserts:
#   - CreateReport returns a UUID reportId
#   - GetReport echoes the title, marks the report as v0 (proto3
#     omits int32 zero-defaults), and exposes an ISO 8601 createdAt
#   - ListReportVersions returns an empty collection (no run yet)
#   - DeleteReport succeeds (no active run)
#   - GetReport on the deleted id returns Connect not_found (HTTP 404)
POST {{base_url}}/alt.acolyte.v1.AcolyteService/CreateReport
Content-Type: application/json
Connect-Protocol-Version: 1
file,e2e/fixtures/acolyte-orchestrator/report-create-no-scope.json;
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
jsonpath "$.report.title" == "hurl-acolyte-no-scope"
jsonpath "$.report.reportType" == "weekly_briefing"
jsonpath "$.report.createdAt" matches "^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}"
jsonpath "$.report.currentVersion" not exists
jsonpath "$.sections" not exists

POST {{base_url}}/alt.acolyte.v1.AcolyteService/ListReportVersions
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "reportId": "{{report_id}}",
  "limit": 10
}
HTTP 200
[Asserts]
jsonpath "$.versions" not exists
jsonpath "$.hasMore" not exists

POST {{base_url}}/alt.acolyte.v1.AcolyteService/DeleteReport
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "reportId": "{{report_id}}"
}
HTTP 200

POST {{base_url}}/alt.acolyte.v1.AcolyteService/GetReport
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "reportId": "{{report_id}}"
}
HTTP 404
[Asserts]
jsonpath "$.code" == "not_found"
