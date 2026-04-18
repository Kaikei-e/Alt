# GetTodayDigest for a fresh user has no row in today_digest_view. The
# handler returns a nil Digest which proto3-JSON serializes as an
# omitted field, so the response body is `{}`.
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/GetTodayDigest
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "userId": "{{user_id}}",
  "date": "{{today}}"
}
HTTP 200
[Asserts]
jsonpath "$.digest" not exists
