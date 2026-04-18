# POST /admin/snapshots/create — full snapshot of all three projection
# tables. Requires max_event_seq > 0, which we satisfy via 03. Asserts
# the immutable-design contract: EventSeqBoundary + SchemaVersion +
# ProjectorBuildRef are recorded alongside the artifacts.
#
# The admin handler marshals `sovereign_db.SnapshotMetadata` via
# encoding/json without tags, so JSON field names are the Go
# PascalCase identifiers (SnapshotID, EventSeqBoundary, etc.).
POST {{metrics_url}}/admin/snapshots/create
HTTP 200
[Captures]
snapshot_id: jsonpath "$.SnapshotID"
[Asserts]
jsonpath "$.SnapshotID" matches "^[0-9a-f-]{36}$"
jsonpath "$.SnapshotType" == "full"
jsonpath "$.Status" == "valid"
jsonpath "$.EventSeqBoundary" > 0
jsonpath "$.SchemaVersion" exists
jsonpath "$.ProjectorBuildRef" == "staging"
jsonpath "$.ItemsChecksum" matches "^sha256:[0-9a-f]+$"
jsonpath "$.DigestChecksum" matches "^sha256:[0-9a-f]+$"
jsonpath "$.RecallChecksum" matches "^sha256:[0-9a-f]+$"
