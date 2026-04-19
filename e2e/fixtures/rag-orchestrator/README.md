# rag-orchestrator E2E fixtures

Committed test data consumed by `e2e/hurl/rag-orchestrator/`. Never reused outside the ephemeral `alt-staging` stack.

## UUIDs

All UUIDs are synthetic sentinel values that never collide with real identifiers minted by `uuid_generate_v4()` (which are always random v4 UUIDs).

| File | Purpose |
|---|---|
| `test-user-id.txt` | `X-Alt-User-Id` for the seeded-data scenarios (`08-*`, `09-*`). |
| `test-empty-user-id.txt` | Distinct user whose conversation list must be empty (`05-*`). |
| `test-conversation-id.txt` | Conversation row seeded by `db-seed.sql`. Referenced by `09-*`. |

## Seed SQL

`db-seed.sql` inserts one `augur_conversations` row for `test-user-id` with two `augur_messages` (user + assistant). `run.sh` passes both UUIDs via `psql -v` so the INSERT statements pick them up.

Seeding happens after Atlas migrations apply. The INSERT uses `ON CONFLICT DO NOTHING` so a `KEEP_STACK=1` debug loop that reuses the volume doesn't fail on re-run.

## Auth model note

`rag-orchestrator` trusts `X-Alt-User-Id` blindly — it doesn't verify a JWT. In production the header is set by `alt-backend` after it validates the user session; the edge proxy is responsible for stripping any inbound `X-Alt-User-Id` from external clients. The Hurl suite injects the header directly because it runs inside `alt-staging` (internal network, no edge).
