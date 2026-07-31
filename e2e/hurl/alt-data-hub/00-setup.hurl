# alt-data-hub readiness probe — must run first. Retries /health on the
# mTLS listener until the compose healthcheck settles.
#
# Every entry in this suite is spoken over TLS with a client certificate;
# run.sh supplies --cacert / --cert / --key on the command line rather than
# per-entry [Options] so a scenario file can never accidentally fall back to
# an anonymous connection and still look green.
#
# alt-data-hub publishes no host port, so this probe only resolves from
# inside the staging network — which is exactly why run.sh runs Hurl in a
# container attached to it (same pattern as mq-hub / knowledge-sovereign).
GET {{data_hub_url}}/health
[Options]
retry: 60
retry-interval: 1000
HTTP 200
[Asserts]
jsonpath "$.status" == "healthy"
jsonpath "$.service" == "alt-data-hub"
