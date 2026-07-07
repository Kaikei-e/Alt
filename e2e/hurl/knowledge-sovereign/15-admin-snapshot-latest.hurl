# GET /admin/snapshots/latest — returns the most recent valid snapshot.
# Must match the one created in 14. Fields are explicit snake_case json
# tags on sovereign_db.SnapshotMetadata (ADR-000942).
GET {{metrics_url}}/admin/snapshots/latest
HTTP 200
[Asserts]
jsonpath "$.status" == "valid"
jsonpath "$.snapshot_type" == "full"
jsonpath "$.event_seq_boundary" > 0
jsonpath "$.snapshot_id" matches "^[0-9a-f-]{36}$"
