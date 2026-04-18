# SelectCurrentLens → GetCurrentLensSelection round-trip. Chained in one
# file so a failure in Select is reported before the Get masks it.
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/SelectCurrentLens
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "selection": {
    "userId": "{{user_id}}",
    "lensId": "{{lens_id}}",
    "lensVersionId": "{{lens_version_id}}",
    "selectedAt": "{{occurred_at}}"
  }
}
HTTP 200


POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/GetCurrentLensSelection
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "userId": "{{user_id}}"
}
HTTP 200
[Asserts]
jsonpath "$.found" == true
jsonpath "$.selection.userId" == "{{user_id}}"
jsonpath "$.selection.lensId" == "{{lens_id}}"
jsonpath "$.selection.lensVersionId" == "{{lens_version_id}}"
