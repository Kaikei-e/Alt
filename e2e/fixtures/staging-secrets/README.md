# Staging-only secrets

Files in this directory are **test fixtures**, not production secrets. They
are read exclusively by `compose/compose.staging.yaml` under the
`alt-staging` project name. Do not reference them from any other compose
stack or script.

The values here are committed to the public repo on purpose — they have no
meaning outside the ephemeral E2E network that only exists while Hurl runs
against it.

## Files

| File | Used by | Role |
|------|---------|------|
| `meili_master_key.txt` | `search-indexer` profile | Meilisearch admin key |
| `alt_backend_token_secret.txt` | `alt-backend`, `auth-hub` profiles | HS256 secret. In the `alt-backend` profile it validates the pre-minted JWT at `e2e/fixtures/alt-backend/test-jwt.txt`; in the `auth-hub` profile it is the signing key for the `X-Alt-Backend-Token` JWT that `/validate` issues and the shared secret that the `/internal/system-user` `X-Internal-Auth` middleware compares against (constant-time). |
| `auth_hub_csrf_secret.txt` | `auth-hub` profile | HMAC-SHA256 secret used by auth-hub's CSRF token generator. Must be ≥ 32 bytes. |
| `auth_hub_kratos_cookie_secret.txt` | `auth-hub` profile | Kratos `secrets.cookie[0]` — HMAC key over `ory_kratos_session` cookies in staging. |
| `auth_hub_kratos_cipher_secret.txt` | `auth-hub` profile | Kratos `secrets.cipher[0]` — exactly 32 bytes (xchacha20-poly1305 requirement). |

Other staging services inline their test passwords as plain env vars
(e.g. `knowledge-sovereign-db` uses `POSTGRES_PASSWORD`) because the
distroless sovereign image cannot `cat` a secrets file. The inlined
values are no more sensitive than the files here.
