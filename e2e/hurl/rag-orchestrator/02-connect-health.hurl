# /connect/health on the Connect-RPC port (:9011, h2c). Static 200 served
# by the Connect server's own mux (server.go:69-73), independent of the
# Augur / MorningLetter handlers — useful for proving the Connect listener
# is up before auth-gated RPCs are probed.
GET {{connect_url}}/connect/health
HTTP 200
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.status" == "healthy"
jsonpath "$.service" == "connect-rpc"
