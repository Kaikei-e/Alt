# alt-backend E2E fixtures

Test fixtures consumed by `e2e/hurl/alt-backend/*.hurl` and the compose
`alt-backend` profile in `compose/compose.staging.yaml`. Every value here is
committed on purpose — these are only meaningful inside the ephemeral
`alt-staging` Docker network.

## Files

| File | Purpose |
|------|---------|
| `test-jwt.txt` | HS256 JWT (role=admin) signed with `../staging-secrets/alt_backend_token_secret.txt`. Passed via the `X-Alt-Backend-Token` header. Issuer `alt-staging-auth-hub`, audience `alt-backend`, sub/tenant_id are fixed non-nil UUIDs, exp=2099-01-01 |
| `sample-feeds.opml` | OPML document with 3 feed URLs — exercised by the `/v1/rss-feed-link/import/opml` multipart scenario |
| `register-feed-1.json` … `register-feed-3.json` | JSON bodies for `POST /v1/rss-feed-link/register` (referenced via `file,` in Hurl) |

## Regenerating the JWT

If the secret in `../staging-secrets/alt_backend_token_secret.txt` ever
rotates, regenerate the token with the matching payload:

```bash
python3 - <<'PY'
import jwt
secret = open('e2e/fixtures/staging-secrets/alt_backend_token_secret.txt').read().strip()
payload = {
    "sub":       "00000000-0000-0000-0000-000000000001",
    "tenant_id": "00000000-0000-0000-0000-000000000001",
    "email":     "e2e-test@alt-staging.invalid",
    "role":      "admin",
    "sid":       "00000000-0000-0000-0000-0000000000ff",
    "iss":       "alt-staging-auth-hub",
    "aud":       "alt-backend",
    "exp":       4070908800,
    "iat":       1700000000,
}
print(jwt.encode(payload, secret, algorithm="HS256"))
PY
```

The compose profile must set `BACKEND_TOKEN_ISSUER=alt-staging-auth-hub` and
`BACKEND_TOKEN_AUDIENCE=alt-backend` so the validator agrees with the token.
