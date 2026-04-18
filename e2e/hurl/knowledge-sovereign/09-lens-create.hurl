# CreateLens — client-assigned lens_id (the RPC takes a full Lens message
# and does not generate an ID server-side, matching the immutable-design
# invariant that UUIDs are stable across reprojection). CreateLensResponse
# is empty on success.
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/CreateLens
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "lens": {
    "lensId": "{{lens_id}}",
    "userId": "{{user_id}}",
    "tenantId": "{{tenant_id}}",
    "name": "hurl-e2e lens {{run_id}}",
    "description": "Hurl E2E lens"
  }
}
HTTP 200
