# GetReport with a syntactically valid but unknown UUID must surface
# a Connect not_found error (HTTP 404, code "not_found"). Same shape
# already exercised by 02-crud-no-scope after a delete; this scenario
# locks the contract for the cold-cache "report never existed" path.
POST {{base_url}}/alt.acolyte.v1.AcolyteService/GetReport
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "reportId": "00000000-0000-0000-0000-000000000000"
}
HTTP 404
[Asserts]
jsonpath "$.code" == "not_found"
