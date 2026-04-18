# RerunSection with an empty section_key must surface Connect
# invalid_argument. Source: connect_service.py:rerun_section guards
# `if not request.section_key: raise ConnectError(INVALID_ARGUMENT, ...)`.
POST {{base_url}}/alt.acolyte.v1.AcolyteService/RerunSection
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "reportId": "00000000-0000-0000-0000-000000000000",
  "sectionKey": ""
}
HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"
