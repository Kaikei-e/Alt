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
| `alt_backend_token_secret.txt` | `alt-backend` profile | HS256 secret used by alt-backend's JWT middleware to validate the test JWT in `e2e/fixtures/alt-backend/test-jwt.txt` |

Other staging services inline their test passwords as plain env vars
(e.g. `knowledge-sovereign-db` uses `POSTGRES_PASSWORD`) because the
distroless sovereign image cannot `cat` a secrets file. The inlined
values are no more sensitive than the files here.
