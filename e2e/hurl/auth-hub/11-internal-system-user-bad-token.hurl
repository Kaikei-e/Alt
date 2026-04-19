# Wrong secret — middleware/internal_auth.go:22-24 distinguishes
# missing (401) from invalid (403). The ConstantTimeCompare happens
# here regardless of length; this scenario exercises the shape, not
# the timing. Timing-safety is verified in
# auth-hub/middleware/internal_auth_test.go.
GET {{auth_hub_url}}/internal/system-user
X-Internal-Auth: wrong-but-length-matched-internal-auth-secret-value-00
HTTP 403
