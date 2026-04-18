# GetReportVersion is intentionally unimplemented (handler raises
# ConnectError(Code.UNIMPLEMENTED)). Connect over HTTP/1.1+JSON maps
# unimplemented to status 501 with body
# `{"code":"unimplemented","message":"..."}`. This scenario locks
# that contract so accidentally enabling a half-built handler shows
# up in CI.
POST {{base_url}}/alt.acolyte.v1.AcolyteService/GetReportVersion
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "reportId": "00000000-0000-0000-0000-000000000000",
  "versionNo": 1
}
HTTP 501
[Asserts]
jsonpath "$.code" == "unimplemented"
