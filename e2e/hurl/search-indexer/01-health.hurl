# search-indexer liveness probe on plaintext :9300.
# Contract: JSON body `{"status":"ok"}` with application/json content-type.
GET http://search-indexer:9300/health
HTTP 200
[Asserts]
jsonpath "$.status" == "ok"
header "Content-Type" contains "application/json"
