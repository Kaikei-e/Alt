# Alt Testing Strategy

## Test Pyramid

```
                 ┌─────────┐
                 │  E2E    │  Playwright (integration + mock)
                ┌┴─────────┴┐
                │  Contract  │  Buf schema + Pact CDC + Proto conformance
               ┌┴───────────┴┐
               │  Component   │  Vitest + Browser (Svelte)
              ┌┴─────────────┴┐
              │    Unit        │  Vitest (TS) / go test (Go) / uv run pytest (Py) / cargo test (Rust)
              └───────────────┘
```

---

## Contract Testing（CDC）

Alt の 23+ マイクロサービスは 5 言語（Go/Python/Rust/TypeScript/Deno）で書かれ、Connect-RPC・HTTP/REST・Redis Streams の 3 プロトコルで通信する。契約テストは 2 層構成で運用する。

### 2 層 Contract Testing 概要

```mermaid
block-beta
  columns 1
  block:layer1["Layer 1: Buf Breaking Change Detection"]
    columns 3
    a1["対象: 全 .proto"]
    a2["検知: フィールド削除・型変更"]
    a3["CI: buf-lint-breaking"]
  end
  space
  block:layer2["Layer 2: Pact CDC (Consumer → Provider)"]
    columns 3
    b1["対象: ランタイム契約"]
    b2["検知: レスポンス形式・ステータス"]
    b3["CI: pact-cdc-go / pact-cdc-rust / pact-cdc-python-consumer<br/>pact-verify-go / pact-verify-python / pact-publish-and-verify"]
  end
```

### サービス間契約マップ

```mermaid
graph LR
  subgraph FE["Frontend"]
    FE_SV["alt-frontend-sv"]
  end

  subgraph BFF
    BF["alt-butterfly-facade"]
  end

  subgraph Backend
    AB["alt-backend"]
    AHARV["alt-harvester"]
    DH["alt-data-hub"]
  end

  subgraph Sovereign
    KS["knowledge-sovereign"]
  end

  subgraph Workers
    PP["pre-processor"]
    RO["rag-orchestrator"]
    RW["recap-worker"]
    RS["recap-subworker"]
    RE["recap-evaluator"]
  end

  subgraph AI
    NC["news-creator"]
    AO["acolyte-orchestrator"]
  end

  subgraph Queue
    MQ["mq-hub"]
    SI["search-indexer"]
    TG["tag-generator"]
  end

  subgraph CLI
    CTL["altctl"]
  end

  FE_SV -->|"Connect-RPC (JSON)<br/>Proto Conformance"| BF
  BF -->|"Connect-RPC (h2c)<br/>✅ Pact CDC"| AB

  AB -->|"Connect-RPC<br/>✅ Buf + Pact"| PP
  AB -->|"Connect-RPC<br/>✅ Pact CDC"| SI
  AB -->|"HTTP/REST<br/>✅ Pact CDC"| RW
  AB -->|"HTTP/REST<br/>✅ Pact CDC"| RO
  AB -->|"Connect-RPC<br/>✅ Pact CDC"| DH
  AHARV -->|"Connect-RPC<br/>✅ Pact CDC"| DH
  AB -->|"Connect-RPC<br/>✅ Pact CDC"| KS

  PP -->|"HTTP/REST<br/>✅ Pact CDC"| NC
  PP -->|"Connect-RPC<br/>✅ Pact CDC<br/>(pact provider name: alt-backend [legacy])"| DH
  PP -.->|"Redis Streams<br/>✅ Pact Message"| MQ

  RO -->|"HTTP/REST<br/>✅ Pact CDC"| NC
  RO -->|"HTTP/REST<br/>✅ Pact CDC"| SI
  RO -->|"HTTP/REST<br/>✅ Pact CDC"| RW
  RO -->|"Connect-RPC<br/>✅ Pact CDC"| DH
  RO -->|"Connect-RPC<br/>✅ Pact CDC"| KS

  RW -->|"HTTP/REST<br/>✅ Pact CDC"| NC
  RW -->|"HTTP/REST<br/>✅ Pact CDC"| RS
  RW -->|"HTTP/REST<br/>✅ Pact CDC"| TG
  RW -->|"Connect-RPC<br/>✅ Pact CDC<br/>(pacts: recap-worker-alt-data-hub &<br/>recap-worker-alt-backend [legacy])"| DH
  RW -->|"Connect-RPC<br/>✅ Pact CDC"| KS

  SI -->|"Connect-RPC<br/>✅ Pact CDC<br/>(pact provider name: alt-backend [legacy])"| DH
  SI -->|"HTTP/REST<br/>✅ Pact CDC"| RW
  SI -.->|"Redis Streams<br/>✅ Pact Message"| MQ

  TG -.->|"Redis Streams<br/>✅ Pact Message"| MQ
  TG -->|"Connect-RPC<br/>✅ Pact CDC<br/>(pact provider name: alt-backend [legacy])"| DH

  RE -->|"HTTP/REST<br/>✅ Pact CDC"| RW

  AO -->|"HTTP/REST<br/>✅ Pact CDC"| NC
  AO -->|"HTTP/REST<br/>✅ Pact CDC"| SI

  CTL -->|"HTTP/REST<br/>✅ Pact CDC"| KS

  style AB fill:#c8e6c9
  style AHARV fill:#c8e6c9
  style DH fill:#c8e6c9
  style KS fill:#c8e6c9
  style PP fill:#c8e6c9
  style RO fill:#c8e6c9
  style RW fill:#c8e6c9
  style RS fill:#c8e6c9
  style RE fill:#c8e6c9
  style NC fill:#c8e6c9
  style AO fill:#c8e6c9
  style MQ fill:#c8e6c9
  style SI fill:#c8e6c9
  style TG fill:#c8e6c9
  style BF fill:#c8e6c9
  style CTL fill:#c8e6c9
```

**凡例:** 矢印: Consumer → Provider（Message Pact はストリーム購読側 → mq-hub）/ ✅ Pact CDC 導入済み / 緑: テスト済み

### Layer 1: Buf スキーマ検証

`.proto` ファイルの破壊的変更を PR レベルで自動検知する。

| 項目 | 値 |
|------|-----|
| ツール | `buf` CLI（Go 製、Java 不要） |
| 設定 | `proto/buf.yaml`（`breaking.use: FILE`） |
| CI ジョブ | `buf-lint-breaking` |
| タイミング | PR + push to main（proto/ 変更時） |

```bash
# ローカル実行
cd proto && buf lint
cd proto && buf breaking --against '.git#branch=main'
```

### Layer 2: Pact CDC

Consumer（呼び出し側）が期待するリクエスト/レスポンス形式を Pact 契約ファイル（JSON）として記録し、Provider（提供側）がそれを満たしていることを自動検証する。

#### 技術構成

| 依存 | バージョン | Java 依存 |
|------|-----------|----------|
| pact-go | v2.4.2 | **不要**（Rust FFI 経由） |
| pact-python | v3.2.1 | **不要**（Rust FFI 経由） |
| pact_consumer (Rust) | v1.4.2 | **不要**（ネイティブ Rust） |
| libpact_ffi | v0.4.28 | Rust 製ネイティブライブラリ |
| Pact Broker | latest | Docker（`compose/pact.yaml`、`pact` profile） |

#### テスト対象ペア

| Consumer | Provider | プロトコル | テスト数 | テストファイル |
|----------|----------|-----------|---------|--------------|
| alt-backend | pre-processor | Connect-RPC (JSON) | 3 | `alt-backend/app/driver/preprocessor_connect/contract/consumer_test.go` |
| pre-processor | news-creator | HTTP/REST | 3 | `pre-processor/app/driver/contract/news_creator_consumer_test.go` |
| rag-orchestrator | news-creator | HTTP/REST (/api/chat) | 2 | `rag-orchestrator/internal/adapter/contract/news_creator_chat_consumer_test.go` |
| recap-worker | news-creator | HTTP/REST | 3 | `recap-worker/recap-worker/src/clients/news_creator/contract.rs` |
| recap-worker | recap-subworker | HTTP/REST | 4 | `recap-worker/recap-worker/src/clients/subworker_contract.rs` |
| recap-worker | alt-backend | HTTP/REST | 1 | `recap-worker/recap-worker/src/clients/alt_backend_contract.rs` |
| recap-worker | tag-generator | HTTP/REST | 2 | `recap-worker/recap-worker/src/clients/tag_generator_contract.rs` |
| search-indexer | alt-backend | Connect-RPC | 2 | `search-indexer/app/driver/contract/backend_consumer_test.go` |
| search-indexer | recap-worker | HTTP/REST | 1 | `search-indexer/app/driver/contract/recap_consumer_test.go` |
| recap-evaluator | recap-worker | HTTP/REST | 3 | `recap-evaluator/tests/contract/test_recap_worker_consumer.py` |
| mq-hub ↔ search-indexer | Redis Streams | Pact Message | — | `mq-hub/app/driver/contract/`, `search-indexer/app/driver/contract/` |
| — | news-creator (Provider) | — | 2 | `news-creator/app/tests/contract/test_provider_verification.py` |
| — | recap-subworker (Provider) | — | — | `recap-subworker/tests/contract/test_provider_verification.py` |
| — | tag-generator (Provider) | — | — | `tag-generator/app/tests/contract/test_provider_verification.py` |
| alt-butterfly-facade | alt-backend | Connect-RPC (h2c proxy) | 3 | `alt-butterfly-facade/internal/handler/contract/backend_consumer_test.go` |
| auth-hub | kratos | HTTP/REST | 3 | `auth-hub/internal/adapter/gateway/contract/kratos_consumer_test.go` |
| — | alt-backend (Provider) | — | — | `alt-backend/app/driver/contract/provider_test.go` |

#### Pact Consumer テスト（Go）

CDC の Consumer テストは `//go:build contract` ビルドタグで通常の `go test ./...` から分離されている。

```bash
# Consumer テスト実行（pact JSON を生成）
cd alt-backend/app && go test -tags=contract ./driver/preprocessor_connect/contract/ -v
cd pre-processor/app && go test -tags=contract ./driver/contract/ -v
cd rag-orchestrator && go test -tags=contract ./internal/adapter/contract/ -v
cd search-indexer/app && go test -tags=contract ./driver/contract/ -v
cd mq-hub/app && go test -tags=contract ./driver/contract/ -v
```

各パッケージには `doc.go` が含まれ、タグなし実行時は `[no test files]`（exit 0）になる。

#### Pact Consumer テスト（Rust）

recap-worker の CDC Consumer テストは `#[ignore]` 属性で通常の `cargo test` から分離されている。

```bash
# Consumer テスト実行（pact JSON を生成）
cd recap-worker/recap-worker && cargo test --lib contract -- --ignored
```

#### Pact Consumer テスト（Python）

```bash
# Consumer テスト実行（pact JSON を生成）
cd recap-evaluator && uv run pytest tests/contract/ -v --no-cov
```

#### Pact Provider 検証テスト（Python）

Consumer テストが生成した pact JSON ファイルを、news-creator の実アプリ（モック依存）で検証する。

```bash
# Provider 検証実行
cd news-creator/app && SERVICE_SECRET=test-secret uv run pytest tests/contract/ -v
```

Provider state に応じて mock の挙動を切り替える:
- `"the LLM queue is full"` → `QueueFullError` → HTTP 429
- その他 → 正常レスポンス → HTTP 200

#### 生成される Pact 契約ファイル

```
alt-backend/pacts/alt-backend-pre-processor.json
pacts/pre-processor-news-creator.json
rag-orchestrator/pacts/rag-orchestrator-news-creator.json
```

#### 検証内容の例

**alt-backend → pre-processor (Summarize):**
- `POST /services.preprocessor.v2.PreProcessorService/Summarize`
- Request: `{"articleId": "...", "title": "...", "content": "..."}`
- Response: `{"success": true, "summary": "...", "articleId": "..."}`

**rag-orchestrator → news-creator (/api/chat streaming):**
- `POST /api/chat`
- Request: `{"model": "gemma3:4b-it-qat", "messages": [...], "stream": true}`
- Response: `Content-Type: application/x-ndjson`

**pre-processor → news-creator (429 Queue Full):**
- `POST /api/v1/summarize` + content >= 100 chars
- Response: HTTP 429 + `Retry-After: 30` + `{"error": "queue full"}`

### Layer 3: FE Proto Conformance（既存）

フロントエンドのモックデータが proto スキーマと乖離していないことを検証する。

```bash
cd alt-frontend-sv && bun test src/test/contracts/
```

| テストファイル | 検証内容 |
|--------------|---------|
| `feed-contract.test.ts` | Feed proto の round-trip + required fields |
| `article-contract.test.ts` | Article proto の conformance |
| `recap-contract.test.ts` | Recap proto の conformance |
| `knowledge-home-contract.test.ts` | Knowledge Home proto の conformance |
| `knowledge-home-admin-contract.test.ts` | KH Admin proto の conformance |
| `rest-v1-contract.test.ts` | REST v1 の Valibot schema 検証 |

---

## CI ワークフロー: proto-contract.yaml

```mermaid
graph TD
  trigger["push / PR to main (対象 24 パス変更)<br/>/ workflow_dispatch (手動実行)"]
  trigger --> buf["buf-lint-breaking<br/>Buf lint + breaking<br/>+ protovis allowlists"]
  trigger --> fe["contract-conformance<br/>FE Proto Conformance<br/>(bun test)"]
  trigger --> go_c["pact-cdc-go<br/>Consumer Tests (Go matrix):<br/>alt-backend/app, pre-processor/app,<br/>rag-orchestrator, search-indexer/app,<br/>mq-hub/app, alt-butterfly-facade, altctl"]
  trigger --> rust_c["pact-cdc-rust<br/>Consumer Tests (Rust):<br/>recap-worker"]
  trigger --> py_c["pact-cdc-python-consumer<br/>Consumer Tests (Python):<br/>recap-evaluator,<br/>acolyte-orchestrator"]

  go_c --> go_v["pact-verify-go<br/>Provider Verify (Go):<br/>alt-backend, knowledge-sovereign,<br/>search-indexer, mq-hub"]
  rust_c --> go_v
  py_c --> go_v

  go_c --> py_v["pact-verify-python<br/>Provider Verify (Python matrix):<br/>news-creator/app, recap-subworker,<br/>tag-generator/app"]
  rust_c --> py_v
  py_c --> py_v

  go_v --> agg["pact-publish-and-verify<br/>Aggregator (required check)"]
  py_v --> agg

  style buf fill:#e3f2fd
  style fe fill:#e3f2fd
  style go_c fill:#c8e6c9
  style rust_c fill:#ffccbc
  style py_c fill:#fff9c4
  style go_v fill:#c8e6c9
  style py_v fill:#fff9c4
  style agg fill:#ede7f6
```

**トリガー条件:** push/PR to main で以下のパスが変更された場合:
- `.github/workflows/proto-contract.yaml`
- `proto/**`
- `alt-frontend-sv/src/lib/gen/**`
- `alt-butterfly-facade/internal/server/allowlist_gen.go`
- `alt-frontend-sv/src/test/contracts/**`
- `alt-backend/app/orchestrator/driver/preprocessor_connect/contract/**`
- `alt-backend/app/dataplane/driver/contract/**`
- `alt-backend/app/shared/gateway/datahub_gateway/contract/**`
- `alt-backend/app/shared/driver/sovereign_client/contract/**`
- `pre-processor/app/driver/contract/**`
- `rag-orchestrator/internal/adapter/contract/**`
- `search-indexer/app/driver/contract/**`
- `mq-hub/app/driver/contract/**`
- `knowledge-sovereign/app/driver/contract/**`
- `news-creator/app/tests/contract/**`
- `recap-subworker/tests/contract/**`
- `tag-generator/app/tests/contract/**`
- `recap-evaluator/tests/contract/**`
- `recap-worker/recap-worker/src/clients/*contract*`
- `recap-worker/recap-worker/src/clients/**/contract.rs`
- `alt-butterfly-facade/internal/handler/contract/**`
- `acolyte-orchestrator/tests/contract/**`
- `scripts/pact-check.sh`
- `scripts/tests/**`

**FFI ライブラリのインストール:** `$HOME/.pact/lib/` にダウンロードし、`LD_LIBRARY_PATH` + `CGO_LDFLAGS` で参照。sudo 不要。

---

## 開発フローへの統合

### TDD ワークフロー（/tdd-workflow スキル）

サービス境界を跨ぐ変更時は、通常の RED→GREEN→REFACTOR の前に Phase 0（CONTRACT CHECK）が挿入される:

```mermaid
flowchart TD
  start(["変更開始"]) --> scope{"変更スコープの判定"}
  scope -->|"内部リファクタ<br/>(UI・境界変更なし)"| p2["Phase 2: RED<br/>Unit failing test (stub)"]
  scope -->|"ユーザ旅程 /<br/>サービス間フロー"| p0["Phase 0: E2E first<br/>Playwright (Browser / API)"]

  p0 --> bound{"サービス境界を<br/>跨ぐ変更?"}
  bound -->|No| p2
  bound -->|Yes| p1["Phase 1: CDC contract check<br/>（非排他チェックリスト・該当全項目を実施）<br/>• Proto: buf lint + buf breaking<br/>• Pact: Consumer 先行 → Provider 検証<br/>• LLM: options / 必須ヘッダー / mTLS 整合性確認"]

  p1 -->|"通常フロー"| p2
  p1 -.->|"Provider 要件厳格化時<br/>(新必須ヘッダー / 認証 / mTLS 昇格)"| p1b["Phase 1b: Provider adds a requirement<br/>全 Consumer の Pact 網羅確認・更新<br/>+ Provider 検証 union 実行"]
  p1b -.-> p2

  p2 --> p3["Phase 3: GREEN<br/>minimal implementation + DI 配線"]
  p3 --> p4["Phase 4: REFACTOR<br/>リファクタ + 境界変更時は CDC 再実行"]
  p4 --> p5["Phase 5: Local CI parity<br/>format / lint / type / security sweep<br/>(+ pact-check.sh)"]
  p5 --> done(["完了"])

  style p0 fill:#e1f5fe
  style p1 fill:#e8eaf6
  style p1b fill:#ede7f6
  style p2 fill:#ffcdd2
  style p3 fill:#c8e6c9
  style p4 fill:#fff9c4
  style p5 fill:#ede7f6
```

### サービス境界チェックリスト

サービス間通信を変更する際に確認する項目（PM-004/006/008 から学んだ教訓）:

- [ ] `buf breaking` が PASS する
- [ ] LLM パラメータが全リクエストパスで一致する（PM-008 防止）
- [ ] GPU リクエストが HybridPrioritySemaphore を経由する（PM-006 防止）
- [ ] プロキシ層が全 Connect-RPC シリアライゼーション形式を検出する（PM-004 防止）
- [ ] CDC Consumer テストが更新されている

---

## その他のテストカテゴリ

### Unit Tests
- **Frontend**: `cd alt-frontend-sv && bun test`
- **Backend (Go)**: `cd <service>/app && go test ./...`
- **Backend (Python)**: `cd <service>/app && SERVICE_SECRET=test-secret uv run pytest tests/ -v`
- **Backend (Rust)**: `cd <service> && cargo test`

### Component Tests (Browser)
- **Command**: `VITEST_BROWSER=true bun run test:client`
- **Location**: `src/**/*.svelte.test.ts`
- **MSW**: Shared handlers from `src/test/msw-setup.ts`

### E2E Tests (Mock)
- **Command**: `cd alt-frontend-sv && bun run test:e2e`
- **Projects**: auth, desktop-chromium, desktop-webkit, mobile-chrome, mobile-safari
- **Location**: `tests/e2e/{auth,desktop,mobile}/`

### E2E Tests (Integration)
- **Command**: `ALT_RUNTIME_URL=http://<IP>:4173/sv/ bun run test:e2e:integration`
- **Location**: `tests/e2e/integration/`

### Visual Regression Tests
- **Command**: `npx playwright test --project=visual-regression`
- **Location**: `tests/e2e/visual/`

### Performance Tests
- **K6**: Weekly CI smoke test (`.github/workflows/performance-smoke.yaml`)
- **Go benchmarks**: `go test -bench=. ./app/performance_tests/`

---

## ディレクトリ構造

```
alt-backend/
  app/
    driver/
      preprocessor_connect/
        contract/               # Pact CDC Consumer (Connect-RPC)
          consumer_test.go      # //go:build contract
          doc.go
    integration_tests/          # Go integration tests
    performance_tests/          # Go benchmarks
  pacts/                        # 生成された Pact JSON
    alt-backend-pre-processor.json

pre-processor/
  app/
    driver/
      contract/                 # Pact CDC Consumer (HTTP/REST)
        news_creator_consumer_test.go  # //go:build contract
        doc.go

rag-orchestrator/
  internal/
    adapter/
      contract/                 # Pact CDC Consumer (/api/chat)
        news_creator_chat_consumer_test.go  # //go:build contract
        doc.go
  pacts/
    rag-orchestrator-news-creator.json

news-creator/
  app/
    tests/
      contract/                 # Pact Provider Verification
        test_provider_verification.py

pacts/                          # 共有 Pact JSON
  pre-processor-news-creator.json

alt-frontend-sv/
  src/
    test/
      contracts/                # FE Proto conformance
        feed-contract.test.ts
        article-contract.test.ts
        ...
  tests/
    e2e/                        # Playwright E2E
      auth/
      desktop/
      mobile/
      integration/
      visual/

compose/
  pact.yaml                     # Pact Broker (profile: pact)

proto/
  buf.yaml                      # Buf 設定（breaking.use: FILE）
  buf.gen.yaml                  # コード生成設定
```

---

## CI ワークフロー一覧

| Workflow | Trigger | テスト内容 |
|----------|---------|-----------|
| `proto-contract.yaml` | proto/ + contract/ 変更 | Buf lint/breaking + FE conformance + Pact CDC (Go x3 + Python) |
| `backend-go.yaml` | alt-backend/ 変更 | Go unit tests (`go test ./...`、CDC は build tag で除外) |
| `alt-frontend-sv.yml` | alt-frontend-sv/ 変更 | Playwright E2E (3 shards) |
| `alt-frontend-sv-unit-test.yaml` | alt-frontend-sv/ 変更 | Vitest unit tests |
| `performance-smoke.yaml` | Weekly | K6 API smoke test |
| `search-indexer.yaml` | search-indexer/ 変更 | Go unit tests |
| `tag-generator.yaml` | tag-generator/ 変更 | Python tests |

---

## 関連 ADR

- [[000588]] マイクロサービス間 Contract Testing 戦略として Buf + Pact CDC の 2 層構成を採用する
- [[000589]] Pact CDC テスト基盤を導入し最重要 3 ペアの Consumer/Provider テストを実装する
- [[000590]] tdd-workflow スキルに CDC Contract Testing フローを統合する
- [[000591]] Pact CDC テストを全サービスペアに全面展開し Pact Broker による契約管理を確立する
- [[000735]] search-indexer を呼ぶ全 consumer に X-Service-Token を強制し Pact で回帰を封じる
- [[000736]] Pact CDC の残ギャップを埋めて Broker を常時稼働・can-i-deploy で docker publish をブロックする
- [[000737]] search-indexer REST を :9443 peer-identity 化し Acolyte sidecar を VERIFY_CLIENT=on へ昇格する
- Runbook: [[pact-broker-ops]] / [[mtls-cutover]]

### 現在の状態 (2026-04-15 時点)

- **Pact Broker**: `compose/pact.yaml` default profile で常時稼働 ([[000736]])。Docker secret 認証、Restic バックアップ対象
- **CI gate**: `.github/workflows/proto-contract.yaml` が PR 必須 gate、`release-gate.yaml` が deploy 前に `can-i-deploy` を強制
- **pact-check.sh**: 15 検証 (consumer + provider) が 0 failed で pass
- **Provider coverage**: alt-backend (3 consumers), search-indexer (3 consumers), mq-hub (1 async), news-creator (4), recap-subworker (1), tag-generator (2)
- **Auth**: REST `/v1/search` は X-Service-Token (Phase C で撤去予定)、Connect-RPC は peer-identity allowlist ([[000737]])
