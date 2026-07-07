# POST /admin/snapshots/create — full snapshot of all three projection
# tables. Requires max_event_seq > 0, which we satisfy via 03. Asserts
# the immutable-design contract: event_seq_boundary + schema_version +
# projector_build_ref are recorded alongside the artifacts.
#
# ADR-000941: the admin handler marshals `sovereign_db.SnapshotMetadata`
# via explicit snake_case json tags (superseding ADR-000765 §3's
# PascalCase-no-tags stance), matching altctl's home_snapshot.go decode
# structs.
POST {{metrics_url}}/admin/snapshots/create
HTTP 200
[Captures]
snapshot_id: jsonpath "$.snapshot_id"
[Asserts]
jsonpath "$.snapshot_id" matches "^[0-9a-f-]{36}$"
jsonpath "$.snapshot_type" == "full"
jsonpath "$.status" == "valid"
jsonpath "$.event_seq_boundary" > 0
jsonpath "$.schema_version" exists
jsonpath "$.projector_build_ref" == "staging"
jsonpath "$.items_checksum" matches "^sha256:[0-9a-f]+$"
jsonpath "$.digest_checksum" matches "^sha256:[0-9a-f]+$"
jsonpath "$.recall_checksum" matches "^sha256:[0-9a-f]+$"
