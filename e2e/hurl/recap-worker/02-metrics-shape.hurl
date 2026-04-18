# /metrics returns Prometheus text exposition format
# (observability.rs:57-60 uses prometheus::TextEncoder). Pin the
# endpoint's contract: 200 + text/plain. Content may be empty until
# the default prometheus registry has samples; the recap-worker
# service wires its counters into a custom registry (metrics.rs:68-175)
# that `prometheus::gather()` does not surface today, so only the
# envelope is asserted here. Tightening the assertion is a separate
# follow-up owned by whoever fixes the registry binding.
GET {{base_url}}/metrics
HTTP 200
[Asserts]
header "Content-Type" contains "text/plain"
