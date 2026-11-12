# alt-backend E2E fixtures

Test fixtures consumed by `e2e/playwright/alt-backend/` and the compose
`alt-backend` profile in `compose/compose.staging.yaml`.

There is no committed token. `e2e/playwright/alt-backend/run.sh` calls
`suite_token_keys`, which mints a throwaway Ed25519 key pair into the
gitignored `token-keys/` directory next to this file, then signs the admin
JWT (EdDSA, role=admin, issuer `alt-staging-auth-hub`, audience `alt-backend`,
fixed non-nil UUID sub/tenant_id, exp=2099-01-01) into
`token-keys/test-jwt.txt`. The compose profile mounts the public half so the
backend verifies it, and must set `BACKEND_TOKEN_ISSUER=alt-staging-auth-hub`
and `BACKEND_TOKEN_AUDIENCE=alt-backend` so the validator agrees with the
token.
