# DiffReportVersions is unimplemented — same contract as scenario 20.
POST {{base_url}}/alt.acolyte.v1.AcolyteService/DiffReportVersions
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "reportId": "00000000-0000-0000-0000-000000000000",
  "fromVersion": 1,
  "toVersion": 2
}
HTTP 501
[Asserts]
jsonpath "$.code" == "unimplemented"
