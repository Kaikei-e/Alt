# Internal service-to-service endpoint. The InternalAuth middleware
# (middleware/internal_auth.go:14-28) compares the X-Internal-Auth
# header against cfg.BackendTokenSecret in constant time. The response
# is the first identity returned by Kratos Admin (usecase.GetSystemUser
# → gateway.ListIdentities). In our seeded staging DB, that's the
# identity we just created in 00-setup, so user_id matches tenant_id.
GET {{auth_hub_url}}/internal/system-user
X-Internal-Auth: {{backend_token_secret}}
# Kratos admin ListIdentities can 5xx briefly on cold boot. The rate
# limit (10 req/min, burst 3, non-env-tunable) is fresh here, so one
# attempt normally suffices; retry-interval is generous enough that a
# cache miss + warm-up can still land inside a single retry window.
[Options]
retry: 3
retry-interval: 6000
HTTP 200
[Asserts]
jsonpath "$.user_id" matches "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"
jsonpath "$.user_id" == "{{user_id}}"
