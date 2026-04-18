# GET /admin/storage/stats — table-level storage metrics. The sovereign
# DB has at least the knowledge_events table populated (from 03), so the
# response is a non-empty array.
GET {{metrics_url}}/admin/storage/stats
HTTP 200
[Asserts]
jsonpath "$" isCollection
