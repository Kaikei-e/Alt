# alt-backend

**Go 1.26+**, **Echo** + **Connect-RPC**, Clean Architecture. 1 ディレクトリ 3 バイナリ。

> Details: `docs/services/alt-backend.md` / 分割の根拠: `docs/ADR/000954.md`

## 3 バイナリ構成 (ADR-000954)

このディレクトリは**単一 Go モジュール** (`alt-backend/app`, `module alt`) だが、
そこから **3 つの独立したコンテナ**をビルドする。同じ `Dockerfile.backend` を
`--build-arg BINARY=<name>` で切り替え、`./cmd/<BINARY>` をビルドする
（`BINARY` 未指定はビルド失敗。「どれかにフォールバック」はしない）。

| compose service | エントリポイント | 責務 | リスナー |
|---|---|---|---|
| `alt-backend` | `cmd/backend` | ユーザ向け API。BFF が叩く面 | REST `:9000` / Connect `:9101` / オペレータ `:9102` / ops `:9110` |
| `alt-harvester` | `cmd/harvester` | `orchestrator/job/` の 7 定期ジョブ | ops `:9110` のみ（業務リスナーなし） |
| `alt-data-hub` | `cmd/datahub` | **alt-db の唯一のオーナー**。`services.datahub.v1.DataHubService` を serve | mTLS `:9443` + ops `:9110`。**publish ゼロ** |

- `cmd/fix_article_titles` は one-shot の運用スクリプトで、常駐サービスではない。
- 共通の起動処理（config ロード / logger / OTel / シグナル / supervisor / ops リスナー）は
  `internal/bootstrap` にあり、3 バイナリで共有する。root `main.go` は存在しない。
- `:9110` は 3 バイナリ共通の ops リスナー（`bootstrap.NewOpsHandler`）で、`/health` と
  `/metrics` だけを出す。Prometheus の scrape job も 3 本ある。

### 内部 API は alt-data-hub にある

- **DB に触るコードは `cmd/datahub` にしか無い。** `cmd/backend` / `cmd/harvester` は
  DB DSN も pgx も持たず、必要なデータは全て `services.datahub.v1.DataHubService`
  （Connect-RPC / mTLS `:9443`）経由で取る。
- これは `di/import_boundary_test.go` が `go list -deps` で**リンクレベルに**強制している。
  `alt/shared/driver/alt_db` と `github.com/jackc/pgx/v5` が `cmd/backend` /
  `cmd/harvester` の依存グラフに現れたらテストが落ちる。grep でも DI 検査でもなく、
  リンカが決める。
- 旧 `services.backend.v1.BackendInternalService` と `/v1/internal/*` REST は**削除済み**。
  同名の面を再追加してはいけない（e2e/playwright の topology スイートが 404 を assert している）。
- `cmd/backend` は mTLS リスナーを一切開かない。`config.RejectBackendMTLSListenerEnv` が
  古い `MTLS_LISTEN` / `MTLS_PORT` の再出現を起動エラーにする。

### DataHubService に capability を足すときの居住規則

> **DB 状態遷移とその不変条件は data-hub へ。外界との対話（外部 HTTP fetch、worker RPC、
> sovereign / mq-hub クライアント）とフロー編成は呼び出し側に残す。**

1 RPC = 1 完結した業務操作 = 1 トランザクション境界。`FOR UPDATE SKIP LOCKED` や
`pg_advisory_xact_lock` が RPC の内側に閉じるように切る。port メソッドを 1:1 で
RPC に写す（ポートミラー型）のは棄却済み — `Begin` と `Commit` が RPC 境界で分断される。

## Commands

```bash
cd alt-backend/app

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

# Container build (BINARY は必須)
docker build --build-arg BINARY=backend -f alt-backend/Dockerfile.backend -t alt-backend:dev alt-backend
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
   （`HostRateLimiter` は `HOST_RATE_LIMITER_REDIS_URL` 設定時に Redis 調停で
   backend / harvester 横断のホスト別スロットを共有する。未設定はプロセス
   ローカル保証のみ = 実効レート最悪 2 倍。詳細は ADR-000954 のレビュー対応節）
3. **Error Wrapping**: Use `fmt.Errorf("context: %w", err)`
4. **Context**: Pass `context.Context` through entire call chain
5. **Logging**: Use `log/slog` with structured context
6. **DB は data-hub だけ**: `cmd/backend` / `cmd/harvester` に DB 依存を足さない
   （`di/import_boundary_test.go` が落ちる）

---

## Clean Architecture

レイヤの向きは全サービス共通:

```
Handler -> Usecase -> Port (interfaces) -> Gateway -> Driver
```

ADR-000945 以降、`app/` 直下ではなく **3 つの package ファミリ**の下にレイヤが並ぶ。
語彙（`rest` / `usecase` / `port` / `gateway` / `driver`）はそのままで、
どのバイナリに入るかが package で決まる。

| package | 主な行き先バイナリ | 中身 |
|---|---|---|
| `app/orchestrator/` | `alt-backend`（`job/` だけ `alt-harvester`） | `rest/` `connect/` `usecase/` `port/` `gateway/` `driver/` `job/` |
| `app/dataplane/` | `alt-data-hub` | `connect/datahubapi` `usecase/` `port/` `gateway/` `driver/` |
| `app/shared/` | 3 バイナリ全部 | `driver/{alt_db,datahub_client,mqhub_connect,sovereign_client}` `gateway/` `port/` `usecase/` |

`app/domain/` `app/config/` `app/middleware/` `app/validation/` `app/utils/` `app/tlsutil/`
`app/adapter/` `app/connect/` `app/gen/` `app/internal/` などは 3 ファミリの外にあり、
必要なバイナリから共有される。

> 注意: 「internal と名の付くものは全部 data-hub」という単純化は backend を壊す。
> `dataplane/gateway/internal_article_gateway` は data-hub 側の capability だけでなく、
> alt-backend 側の `RecallRailUsecase` からも read port として使われている。

### 1. Handler Layer

HTTP entrypoint. Request validation, response formatting.

- REST: `app/orchestrator/rest/`（`routes.go` が Echo のミドルウェアとルート登録）
- Connect: `app/orchestrator/connect/` と `app/connect/v2/`
- data plane: `app/dataplane/connect/datahubapi/`（`services.datahub.v1.DataHubService` の実装）

**Pattern:**
```go
func RestHandleFetchSingleFeed(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
    return func(c echo.Context) error {
        feed, err := container.FetchSingleFeedUsecase.Execute(c.Request().Context())
        if err != nil {
            return HandleError(c, err, "fetch_single_feed")
        }
        return c.JSON(http.StatusOK, feed)
    }
}
```

**Can import:** Usecase, Port, DI Container

### 2. Usecase Layer

Business logic orchestration. No external dependencies.

`app/orchestrator/usecase/` / `app/dataplane/usecase/` / `app/shared/usecase/`

**Pattern:**
```go
type FetchSingleFeedUsecase struct {
    fetchSingleFeedPort fetch_feed_port.FetchSingleFeedPort
}

func NewFetchSingleFeedUsecase(port fetch_feed_port.FetchSingleFeedPort) *FetchSingleFeedUsecase {
    return &FetchSingleFeedUsecase{fetchSingleFeedPort: port}
}

func (u *FetchSingleFeedUsecase) Execute(ctx context.Context) (*domain.RSSFeed, error) {
    return u.fetchSingleFeedPort.FetchSingleFeed(ctx)
}
```

**Can import:** Port only (+ domain, utils/errors)

### 3. Port Layer

Interface definitions (contracts).

`app/orchestrator/port/` / `app/dataplane/port/` / `app/shared/port/`

**Pattern:**
```go
type FetchSingleFeedPort interface {
    FetchSingleFeed(ctx context.Context) (*domain.RSSFeed, error)
}
```

**Can import:** Domain only

### 4. Gateway Layer

Anti-corruption layer. External service boundary.

`app/orchestrator/gateway/` / `app/dataplane/gateway/` / `app/shared/gateway/`

alt-backend / alt-harvester 側の gateway は `shared/driver/datahub_client`
（DataHubService の Connect クライアント）を叩く。DB を直接叩く gateway は
data-hub 側にしか無い。

**Can import:** Port (implements), Driver, Domain

### 5. Driver Layer

Database, external APIs, infrastructure.

- `app/shared/driver/alt_db/` — PostgreSQL repository（**data-hub 専用**）
- `app/shared/driver/datahub_client/` — DataHubService クライアント（backend / harvester 用）
- `app/shared/driver/mqhub_connect/`, `app/shared/driver/sovereign_client/`
- `app/dataplane/driver/kratos_client/`
- `app/orchestrator/driver/` — search-indexer / recap-worker / pre-processor などへの HTTP

**Can import:** External libraries only

### 6. Domain Layer (`app/domain/`)

Core business entities. **Can import:** Nothing (pure Go)

## DI Containers (`app/di/`)

バイナリごとに composition root が分かれている。

| ファイル | 構築関数 | バイナリ |
|---|---|---|
| `di/container_backend.go` | `NewBackendComponents` → `ApplicationComponents`（型は `di/container.go`） | `cmd/backend` |
| `di/container_harvester.go` | `NewHarvesterComponents` → `HarvesterComponents` | `cmd/harvester` |
| `di/datahub/container.go` | data plane の composition root | `cmd/datahub` |

配線の向きは常に `Driver -> Gateway (implements Port) -> Usecase -> Handler`。

Rule 8 に従い、**未配線の依存を握り潰さない**。producer / projector / resolver の
経路で `if x == nil { return nil }` を書かない — 配線忘れと意図的な無効化が
見分けられなくなる。

## Layer Violations (AVOID)

| Violation | Fix |
|-----------|-----|
| Handler -> Driver | Go through Usecase + Gateway |
| Usecase -> external pkg | Create Port interface |
| Gateway -> Usecase | Gateway only implements Port |
| Circular dependency | Extract to new Port |
| `cmd/backend` / `cmd/harvester` から DB | DataHubService の capability を足す |

## References

- [Three Dots Labs - Clean Architecture](https://threedots.tech/post/introducing-clean-architecture/)
- [bxcodec/go-clean-arch](https://github.com/bxcodec/go-clean-arch)
