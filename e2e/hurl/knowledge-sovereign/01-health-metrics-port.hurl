# /health on the metrics port (:9501). This is the port probed by the
# container healthcheck, and it must return the same schema as :9500.
GET {{metrics_url}}/health
HTTP 200
[Asserts]
jsonpath "$.status" == "ok"
jsonpath "$.service" == "knowledge-sovereign"
