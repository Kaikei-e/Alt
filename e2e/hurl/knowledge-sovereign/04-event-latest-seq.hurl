# GetLatestEventSeq must reflect the event_seq captured from 03. The
# handler requires tenant_id and parses user_id via uuid.Parse, so both
# must be valid UUIDs (ADR-000749 tenant-boundary contract).
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/GetLatestEventSeq
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "tenantId": "{{tenant_id}}",
  "userId": "{{user_id}}"
}
HTTP 200
[Asserts]
jsonpath "$.eventSeq" matches "^[0-9]+$"
