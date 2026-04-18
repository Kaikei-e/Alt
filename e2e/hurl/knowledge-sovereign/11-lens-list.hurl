# ListLenses must include the lens created in 09. The query filters on
# user_id and excludes archived rows (migration 00001 idx partial).
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/ListLenses
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "userId": "{{user_id}}"
}
HTTP 200
[Asserts]
jsonpath "$.lenses" count == 1
jsonpath "$.lenses[0].lensId" == "{{lens_id}}"
jsonpath "$.lenses[0].userId" == "{{user_id}}"
jsonpath "$.lenses[0].tenantId" == "{{tenant_id}}"
jsonpath "$.lenses[0].name" == "hurl-e2e lens {{run_id}}"
