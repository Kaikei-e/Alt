# GET /admin/retention/eligible — flat list of partitions eligible for
# archive per the default RetentionPolicy, wrapped in `{"partitions": [...]}`
# per altctl's home_retention.go decode struct (ADR-000941).
#
# Deliberately NOT asserting an exact count here. 00006/00007's monthly
# partitions are hardcoded through 2026-05-01 with no automated extension
# job; whether "today" falls inside that hot window (few/no eligible
# partitions) or past it (all of them eligible) is a partition-maintenance
# concern, not a wire-format one — asserting a specific count made the
# pre-ADR-000941 grouped-by-table shape look stable only because it always
# had exactly 2 top-level entries regardless of eligibility, silently
# hiding this same date drift. The flat shape's field-level contract is
# what this scenario pins.
GET {{metrics_url}}/admin/retention/eligible
HTTP 200
[Asserts]
jsonpath "$.partitions" isCollection
