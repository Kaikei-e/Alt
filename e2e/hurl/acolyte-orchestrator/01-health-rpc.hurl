# Connect-RPC HealthCheck — orthogonal to the REST /health probe. This
# verifies the AcolyteConnectService route table is wired (the FQSN
# resolves) and proto3-JSON marshalling round-trips. HealthCheckRequest
# has no fields, so the body is `{}`.
POST {{base_url}}/alt.acolyte.v1.AcolyteService/HealthCheck
Content-Type: application/json
Connect-Protocol-Version: 1
{}
HTTP 200
[Asserts]
jsonpath "$.status" == "ok"
