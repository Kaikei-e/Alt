# GET /admin/snapshots/list — all snapshots, wrapped in
# `{"snapshots": [...]}` per altctl's home_snapshot.go decode struct
# (ADR-000941). Before this fix the handler returned a bare top-level
# JSON array of PascalCase-keyed objects, which `altctl home snapshot
# list` could never unmarshal into its `{"snapshots": [...]}` +
# snake_case expectation (hard failure, not silent). 14 guarantees at
# least one snapshot exists.
GET {{metrics_url}}/admin/snapshots/list
HTTP 200
[Asserts]
jsonpath "$.snapshots" isCollection
jsonpath "$.snapshots" count > 0
jsonpath "$.snapshots[0].snapshot_id" matches "^[0-9a-f-]{36}$"
jsonpath "$.snapshots[0].status" == "valid"
jsonpath "$.snapshots[0].event_seq_boundary" > 0
