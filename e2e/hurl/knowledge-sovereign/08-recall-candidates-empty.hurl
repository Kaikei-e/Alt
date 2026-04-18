# GetRecallCandidates for a fresh user returns an empty list. Phase 1
# validates the contract shape; recall scoring requires projector +
# signals which are out of scope for the sovereign-only E2E.
# proto3-JSON omits empty repeated fields so the response is `{}`.
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/GetRecallCandidates
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "userId": "{{user_id}}",
  "limit": 10
}
HTTP 200
[Asserts]
jsonpath "$.candidates" not exists
