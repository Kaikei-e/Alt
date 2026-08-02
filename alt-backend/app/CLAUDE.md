# alt-backend/app/CLAUDE.md

## Overview

**Go 1.26+**, **Echo** + **Connect-RPC**, Clean Architecture.

このディレクトリは**単一 Go モジュール** (`module alt`) だが、そこから
**3 つの独立したコンテナ**をビルドする（ADR-000954）。

| バイナリ | compose service | 責務 | リスナー |
|---|---|---|---|
| `cmd/backend` | `alt-backend` | ユーザ向け API。BFF が叩く面 | REST `:9000` / Connect `:9101` / オペレータ `:9102`（admin Connect 2 サービス、loopback）/ ops `:9110` |
| `cmd/harvester` | `alt-harvester` | `orchestrator/job/` の 7 定期ジョブ | ops `:9110` のみ |
| `cmd/datahub` | `alt-data-hub` | **alt-db の唯一のオーナー**。`services.datahub.v1.DataHubService` を serve | mTLS `:9443` + ops `:9110`。publish ゼロ |

`cmd/fix_article_titles` は one-shot の運用スクリプト。root `main.go` は無い。
共通の起動処理は `internal/bootstrap` にあり 3 バイナリで共有する。

**内部 API は alt-data-hub にある。** `cmd/backend` / `cmd/harvester` は DB DSN も
pgx も持たず、データは全て `services.datahub.v1.DataHubService`（Connect-RPC / mTLS）
経由で取る。これは `di/import_boundary_test.go` が `go list -deps` でリンクレベルに
強制している。旧 `services.backend.v1.BackendInternalService` と `/v1/internal/*`
REST は削除済みで、再追加してはいけない。

レイヤの配置・居住規則・DI の分かれ方は `alt-backend/CLAUDE.md` を見ること。

> Details: `docs/services/alt-backend.md` / `docs/ADR/000954.md`

## Commands

```bash
# Test (TDD first) — 3 バイナリ分をまとめて回す
go test ./...

# Coverage
go test -race -cover ./...

# Mocks
make generate-mocks

# Build all three
go build ./cmd/...

# Run one
go run ./cmd/backend      # or ./cmd/harvester, ./cmd/datahub
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Usecase**: Mock ports, test business logic
- **Gateway**: Mock drivers, test external calls
- **Handler**: Use `httptest`, mock usecases
- **新しい capability**: Rule 7 に従い、producer の GREEN より先に Pact CDC RED を置く

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Rate Limiting**: YOU MUST enforce 5-second minimum for external APIs
3. **Error Wrapping**: Use `fmt.Errorf("context: %w", err)`
4. **Context**: Pass `context.Context` through entire call chain
5. **Logging**: Use `log/slog` with structured context
6. **DB は data-hub だけ**: `cmd/backend` / `cmd/harvester` に DB 依存を足さない
