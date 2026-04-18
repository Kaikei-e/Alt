# GET /admin/retention/eligible — lists partitions eligible for archive
# per the default RetentionPolicy. With fresh staging data the list is
# expected to be empty (today's partition is "hot"), but both tables
# must appear in the result so the contract shape is fixed.
GET {{metrics_url}}/admin/retention/eligible
HTTP 200
[Asserts]
jsonpath "$" count == 2
jsonpath "$[0].table" == "knowledge_events"
jsonpath "$[1].table" == "knowledge_user_events"
