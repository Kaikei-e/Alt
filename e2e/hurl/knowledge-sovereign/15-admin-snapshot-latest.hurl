# GET /admin/snapshots/latest — returns the most recent valid snapshot.
# Must match the one created in 14. Fields are PascalCase (no JSON tags
# on sovereign_db.SnapshotMetadata).
GET {{metrics_url}}/admin/snapshots/latest
HTTP 200
[Asserts]
jsonpath "$.Status" == "valid"
jsonpath "$.SnapshotType" == "full"
jsonpath "$.EventSeqBoundary" > 0
jsonpath "$.SnapshotID" matches "^[0-9a-f-]{36}$"
