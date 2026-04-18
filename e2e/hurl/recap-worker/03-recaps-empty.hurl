# Both windowed read endpoints return 404 when no completed job exists.
# See get_recap_by_window in api/fetch.rs:407-418 — Ok(None) →
# (StatusCode::NOT_FOUND, Json(ErrorResponse { error: "No <label> recap found" })).
# Run before any trigger scenario so the empty-DB shape is observable.
GET {{base_url}}/v1/recaps/7days
HTTP 404
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.error" == "No 7-day recap found"

GET {{base_url}}/v1/recaps/3days
HTTP 404
[Asserts]
header "Content-Type" contains "application/json"
jsonpath "$.error" == "No 3-day recap found"
