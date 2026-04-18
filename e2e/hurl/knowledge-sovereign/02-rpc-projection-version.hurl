# GetActiveProjectionVersion — Connect-RPC baseline. The sovereign DB
# seeds `knowledge_projection_versions` with version 1 ACTIVE in
# migration 00001, so a freshly migrated staging stack must return it.
# Establishes that the Connect-RPC JSON surface is reachable before any
# event-mutating scenario runs.
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/GetActiveProjectionVersion
Content-Type: application/json
Connect-Protocol-Version: 1
{}
HTTP 200
[Asserts]
jsonpath "$.version.version" == 1
jsonpath "$.version.status" == "active"
jsonpath "$.version.description" exists
