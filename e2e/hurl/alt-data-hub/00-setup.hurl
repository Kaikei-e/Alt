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
# `connect-rpc`, not `alt-data-hub`. This is the health endpoint every
# Connect-RPC listener in the module mounts (connect/v2/muxutil.RegisterHealth),
# and it names the surface rather than the process — the same body comes back
# from alt-backend:9101 and :9102. The per-binary name lives on the ops
# listener instead, where 03-ops-listener.hurl pins it to `alt-data-hub`.
jsonpath "$.service" == "connect-rpc"
