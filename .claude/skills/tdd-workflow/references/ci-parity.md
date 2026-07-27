# Local CI Parity Commands (Phase 5)

Per-service gates matching what the reusable CI workflows run
(`reusable-test-go.yaml`, `reusable-go-quality-gates.yaml`, `reusable-test-python.yaml`,
`reusable-test-rust.yaml`, `alt-frontend-sv-unit-test.yaml`, `proto-contract.yaml`).
Load this at Phase 5, once you know which services the change touched.
Update this file when CI changes.

- [Go](#go) · [Python](#python) · [Rust](#rust) · [TypeScript / Svelte](#typescript--svelte) · [Deno](#deno)
- [Proto](#proto) · [Database migration](#database-migration) · [Contract regression](#contract-regression)
- [Reporting](#reporting)

## Go

Services: alt-backend, search-indexer, pre-processor, mq-hub, auth-hub, rag-orchestrator, alt-butterfly-facade.

```bash
cd <service>/app      # or the service root for rag-orchestrator / auth-hub
gofmt -l . | grep -v '^gen/' | grep -v '^$'   # must print nothing
go vet ./...
# golangci-lint v2.1 (CI uses golangci-lint-action@v8); install locally with:
#   go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.0
golangci-lint run ./...
go test ./... -race                           # CI uses CGO_ENABLED=1
CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v    # if any CDC changed
```

## Python

Services: acolyte-orchestrator, news-creator, tag-generator, recap-subworker, metrics, recap-evaluator.

```bash
cd <service>/app      # or the service root
uv sync --all-extras --dev
uv run ruff check .
uv run ruff format --check .
uv run pyrefly check
uv run pytest         # CI adds --cov=. --cov-report=xml --junit-xml=tests/results.xml
uv run pytest tests/contract/ -v --no-cov     # if any CDC changed
# news-creator / recap-subworker / tag-generator contract tests need:
#   SERVICE_SECRET=test-secret uv run pytest tests/contract/ -v
```

## Rust

Services: rask-log-aggregator, rask-log-forwarder, recap-worker.

```bash
cd <service>
cargo fmt --all -- --check
cargo clippy --all-targets --all-features -- -D warnings
cargo build --release
cargo test --all
cargo test --lib contract -- --ignored        # if any CDC changed (recap-worker)
```

## TypeScript / Svelte

```bash
cd alt-frontend-sv
bun install --frozen-lockfile
bun run check                  # svelte-check + tsc
bun run lint                   # eslint / prettier --check
bun test                       # vitest unit + contract tests
bun test src/test/contracts/   # if any CDC changed
bun run test:e2e:integration   # Playwright, if UI was touched
```

## Deno

Services: auth-token-manager, alt-perf.

```bash
cd <service>
deno fmt --check
deno lint
deno check <entrypoint>.ts
deno test --allow-all
```

## Proto

Run whenever `proto/**` was touched, regardless of which services changed.

```bash
cd proto
buf lint
buf breaking --against '.git#branch=main'
# Regenerate stubs for every consumer of the changed proto file and commit:
buf generate --template buf.gen.backend-internal.yaml        # alt-backend
buf generate --template buf.gen.pre-processor-services.yaml  # pre-processor
buf generate --template buf.gen.search-indexer.yaml          # search-indexer
# ... and the other buf.gen.<service>.yaml templates as needed
```

## Database migration

Run when `migrations-atlas/migrations/**` was touched.

```bash
cd migrations-atlas
atlas migrate hash            # refresh atlas.sum after new .sql files
atlas migrate validate        # schema is consistent
atlas migrate lint --latest 1 # CI-equivalent linter check
```

## Contract regression

Run when any consumer or provider interaction was touched.

```bash
./scripts/pact-check.sh       # file-based mode, fails closed
```

## Reporting

State which services' gates ran and their exit status. A truthful summary looks like:

> Ran local CI parity for: acolyte-orchestrator (ruff/pyrefly/pytest green, 591 tests),
> alt-backend (gofmt/vet/golangci-lint/go test green), pre-processor (same),
> search-indexer (same). Proto gate green. No CDC regression.

Name any gate you skipped and why (e.g. "skipped golangci-lint locally — not installed, relying on CI").
A silently skipped gate reads as a passing gate, which is how "green locally, red in CI" happens.
