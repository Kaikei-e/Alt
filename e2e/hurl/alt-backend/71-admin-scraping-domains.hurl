# Admin scraping-domain policies at /v1/admin/scraping-domains.

GET {{base_url}}/v1/csrf-token
HTTP 200
[Captures]
csrf: jsonpath "$.csrf_token"

GET {{base_url}}/v1/admin/scraping-domains
X-Alt-Backend-Token: {{jwt}}
HTTP 200
[Asserts]
jsonpath "$" isCollection

# GetByID returns (nil, nil) on no matching row, and the handler maps a
# nil domain straight to 404 (scraping_domain_handlers.go:97-107;
# scraping_domain_driver.go:137-140 turns pgx.ErrNoRows into (nil, nil)
# rather than propagating an error) — deterministic, not "accept both".
GET {{base_url}}/v1/admin/scraping-domains/00000000-0000-0000-0000-000000000000
X-Alt-Backend-Token: {{jwt}}
HTTP 404
[Asserts]
jsonpath "$.error" == "scraping domain not found"

# KNOWN BUG (recorded, not fixed — production code is out of scope for
# this suite): UpdatePolicy's driver DOES check RowsAffected == 0 and
# returns an error for a nonexistent id (scraping_domain_driver.go:
# 339-342), unlike GetByID's (nil, nil) pattern above — but that error is
# a bare `errors.New`, not an AppContextError/AppError, so HandleError
# falls through to its generic UNKNOWN_ERROR branch and answers 500
# instead of 404 (utils/errors/app_context_error.go:42-61 has no
# NOT_FOUND case to hit even if it were typed). GET and PATCH on the same
# nonexistent id therefore disagree on status code. This assertion pins
# the real, current behavior so a regression is still caught; see
# bugs_found in the test-quality report for the suggested fix.
PATCH {{base_url}}/v1/admin/scraping-domains/00000000-0000-0000-0000-000000000000
X-Alt-Backend-Token: {{jwt}}
X-CSRF-Token: {{csrf}}
Content-Type: application/json
{
  "force_respect_robots": true
}
HTTP 500
[Asserts]
jsonpath "$.error.code" == "UNKNOWN_ERROR"
