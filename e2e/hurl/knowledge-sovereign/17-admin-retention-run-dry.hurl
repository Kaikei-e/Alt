# POST /admin/retention/run with dry_run=true. The handler's safety
# check requires a valid snapshot; 14 satisfies this. With no eligible
# partitions in a fresh stack the actions list may be empty, but the
# dry_run flag and absence of `error` are mandatory contract bits.
POST {{metrics_url}}/admin/retention/run
Content-Type: application/json
{
  "dry_run": true
}
HTTP 200
[Asserts]
jsonpath "$.dry_run" == true
jsonpath "$.error" not exists
