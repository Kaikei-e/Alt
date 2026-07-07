# GET /admin/storage/stats — table-level storage metrics, wrapped in
# `{"tables": [...]}` per altctl's home_storage.go decode struct
# (ADR-000942). The sovereign DB has at least the knowledge_events table
# populated (from 03), so the response is a non-empty array with
# table_size/index_size broken out alongside the combined total_size.
GET {{metrics_url}}/admin/storage/stats
HTTP 200
[Asserts]
jsonpath "$.tables" isCollection
jsonpath "$.tables" count > 0
jsonpath "$.tables[0].name" exists
jsonpath "$.tables[0].total_size" exists
jsonpath "$.tables[0].table_size" exists
jsonpath "$.tables[0].index_size" exists
jsonpath "$.tables[0].row_count" exists
