# ListReports pagination shape — creates two reports and asserts both
# appear in the listing. Cleanup at the end keeps subsequent runs of
# the suite from accumulating rows in acolyte-db across reruns when
# KEEP_STACK=1.
POST {{base_url}}/alt.acolyte.v1.AcolyteService/CreateReport
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "title": "hurl-list-1",
  "reportType": "weekly_briefing"
}
HTTP 200
[Captures]
report_id_a: jsonpath "$.reportId"

POST {{base_url}}/alt.acolyte.v1.AcolyteService/CreateReport
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "title": "hurl-list-2",
  "reportType": "tech_review"
}
HTTP 200
[Captures]
report_id_b: jsonpath "$.reportId"

POST {{base_url}}/alt.acolyte.v1.AcolyteService/ListReports
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "limit": 50
}
HTTP 200
[Asserts]
jsonpath "$.reports" isCollection
jsonpath "$.reports" count >= 2
jsonpath "$.reports[*].reportId" contains "{{report_id_a}}"
jsonpath "$.reports[*].reportId" contains "{{report_id_b}}"
jsonpath "$.reports[*].title" contains "hurl-list-1"
jsonpath "$.reports[*].title" contains "hurl-list-2"

POST {{base_url}}/alt.acolyte.v1.AcolyteService/DeleteReport
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "reportId": "{{report_id_a}}"
}
HTTP 200

POST {{base_url}}/alt.acolyte.v1.AcolyteService/DeleteReport
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "reportId": "{{report_id_b}}"
}
HTTP 200
