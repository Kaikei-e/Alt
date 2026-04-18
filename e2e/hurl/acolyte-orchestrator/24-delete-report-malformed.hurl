# DeleteReport with a malformed UUID must surface Connect
# invalid_argument. Source: connect_service.py:340-346 wraps
# UUID(...) in try/except and translates ValueError to
# ConnectError(Code.INVALID_ARGUMENT).
POST {{base_url}}/alt.acolyte.v1.AcolyteService/DeleteReport
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "reportId": "not-a-uuid"
}
HTTP 400
[Asserts]
jsonpath "$.code" == "invalid_argument"
