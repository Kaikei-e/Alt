# /csrf without a cookie — csrf.go:34-40 returns 401 directly (it
# checks the raw Cookie header rather than c.Cookie, so a completely
# missing header is the trigger here).
POST {{auth_hub_url}}/csrf
HTTP 401
