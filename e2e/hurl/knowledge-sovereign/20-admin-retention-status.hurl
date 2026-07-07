# GET /admin/retention/status — retention log, wrapped in
# `{"logs": [...]}` per altctl's home_retention.go decode struct
# (ADR-000941). Before this fix the handler returned a bare top-level
# array (or JSON `null` when the log table is empty), which
# `altctl home retention status` could never unmarshal into its
# `{"logs": [...]}` expectation (hard unmarshal error whenever any log
# rows exist; a JSON `null` decodes silently into a zero-value struct
# otherwise, hiding the drift). 17 only runs `dry_run: true`, which never
# inserts a knowledge_retention_log row, so this scenario can only assert
# the envelope shape, not populated log fields — the same limitation
# already documented for 16.
GET {{metrics_url}}/admin/retention/status
HTTP 200
[Asserts]
jsonpath "$.logs" isCollection
jsonpath "$.logs" count == 0
