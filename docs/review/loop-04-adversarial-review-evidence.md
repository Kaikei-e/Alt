---
report_version: "1.0"
task_id: "LOOP04-2026-11-28-R2"
target: "alt-backend/app, altctl, auth-hub, pre-processor/app, search-indexer/app, rag-orchestrator, news-creator/app, recap-subworker, recap-worker"
base_commit: "123489cad497d8527473a8c2511fe6930a0e3152"
head_commit: "f70479f931f6641d20b034a0f6c3deac04fc4ac3"
---

# LOOP-04 敵対的レビュー実測証拠台帳: LOOP04-2026-11-28-R2

## 1. 変更概要

- 対象サービス / モジュール: `alt-backend/app`, `altctl`, `auth-hub`, `pre-processor/app`, `search-indexer/app`, `rag-orchestrator`, `news-creator/app`, `recap-subworker`, `recap-worker`
- 監査モデル体制: 実装側 `gemini-4-neon-xhigh`（`ADR-001040`、6 件の既存不具合 `REV-05` / `REV-06` / `REV-08` / `REV-03` / `REV-04` / `REV-09` + `REV-01` および Round-1 監査で追加検出した `AUD-01` / `AUD-02` を TDD 是正）に対し、別モデル系統 `gemini-4-carbon-high`（read-only 敵対的レビュアー、`auto_applied: false`）が 2 ラウンドにわたり LOOP-04 重点 7 項目（`F-01`〜`F-07`）を実コード・実テスト駆動（`/interrogate` + `/blast-radius`）で検証した。
- 判定集計: **7 / 7 `safe`**（`F-01`, `F-02`, `F-03`, `F-04`, `F-05`, `F-06`, `F-07` すべて `exit_code == 0` 実行証明つきで確定）。
- 指摘 4 分類（Round-2 確定時点 全 7 件）: `fix` **0 件**、`investigate` **0 件**、`note` 3 件（`AUD-03`, `AUD-04`, `REV-07`）、`dismiss` 4 件（`AUD-05`, `AUD-06`, `AUD-07`, `AUD-08`）。

## 2. 主張と証拠（VERIFY-06 必須）

| 主張 | 証拠（コマンドと出力の保存先） |
| --- | --- |
| `F-01` (`safe`): `errorlint` 書き換え 4 箇所（`C-01` `>= 500` 条件・`C-02` `!errors.As` 否定）の意味論が維持され、`REV-01` の `altctl/internal/config` 単体テストが `!errors.As -> errors.As` 変異を 3 テストで検知する | コマンド: `go -C alt-backend/app test -race -count=1 ./middleware/... ./connect/errorhandler/... ./orchestrator/rest/rest_feeds/summarization/... && go -C altctl test -race -count=1 ./internal/config/... ./cmd/...` / 保存先: `docs/review/loop-04-adversarial-review-report.json` |
| `F-02` (`safe`): `pre-processor` / `search-indexer` / `alt-backend` の全単項 Connect-RPC クライアントが `connectguard.NewUnaryNilResponseInterceptor()` を配線し `C-04` ast-grep 0 件、かつ `REV-03` の `timestampOrZero` により `search-indexer` 全 `.AsTime()` が nil 時にゼロ値 `time.Time{}` を維持する | コマンド: `ast-grep scan -c scripts/lint/ast-grep/sgconfig.yml pre-processor/app search-indexer/app alt-backend/app && go -C search-indexer/app test -race -count=1 ./driver/backend_api/... ./gateway/... ./usecase/... ./internal/connectguard/...` / 保存先: `docs/review/loop-04-adversarial-review-report.json` |
| `F-03` (`safe`): `REV-04`・`AUD-02` 是正により `auth-hub` の全 8 `<KEY>_FILE` 経路が不在・`..`・symlink 脱出・0 バイト・空白のみファイルで秘密値漏洩なしに fail-fast 起動失敗し、`httpserver.New` の `ReadHeaderTimeout=10s`（無断 `IdleTimeout` なし、`C-11`）を維持する | コマンド: `go -C auth-hub test -race -count=1 ./...` / 保存先: `auth-hub/cmd/auth-hub/testdata/effective_config.golden.json` |
| `F-04` (`safe`): `REV-05`・`REV-06`・`AUD-01` 是正により `altctl` の `Manifest.Verify` / `resolveUnderBase` / `loadManifestInDir` / `CreateEnvFile` / `GenerateSecrets` / `ensureEd25519KeyPair` がすべて `os.OpenRoot` + `Lstat` で symlink 脱出・非通常ファイル・権限緩和を遮断し全再現プローブが通過する | コマンド: `go -C altctl test -race -count=1 ./...` / 保存先: `docs/review/loop-04-adversarial-review-report.json` |
| `F-05` (`safe`): 本番コードの `//nolint:nilerr` 全 10 箇所（`alt-backend/app` 9、`rag-orchestrator` 1）が構造化ログ出力＋明示的なドメイン縮退値を返し対象テストが通過する | コマンド: `go -C alt-backend/app test -count=1 ./utils/html_parser/... ./shared/driver/redis_slot_store/... ./orchestrator/usecase/record_reading_events_usecase/... ./orchestrator/gateway/admin_metrics_gateway/... ./orchestrator/usecase/register_favorite_feed_usecase/... ./orchestrator/gateway/csrf_token_gateway/... ./orchestrator/gateway/og_resolve_gateway/... ./orchestrator/gateway/robots_txt_gateway/... && go -C rag-orchestrator test -count=1 ./internal/backfill/...` / 保存先: `docs/review/loop-04-adversarial-review-report.json` |
| `F-06` (`safe`): `REV-08` 是正により `OllamaGateway.chat_stream` が `async with aclosing(stream_iterator)` で切断・例外時に上流ストリームを閉じてから HIGH PRIORITY セマフォ枠を解放し、`HybridPrioritySemaphore` (`C-06`)・`sse_stream.py` (`C-07`) を含む 445 テストが通過する | コマンド: `cd news-creator/app && uv run --offline pytest tests/gateway/ tests/handler/test_sse_stream.py tests/handler/test_chat_handler.py tests/handler/test_summarize_handler.py tests/usecase/test_recap_summary_usecase.py tests/usecase/test_plan_query_usecase.py -q` / 保存先: `docs/review/loop-04-adversarial-review-report.json` |
| `F-07` (`safe`): `REV-09` 是正により `pseudo_label.py` の Python 3.14 `argparse` 未エスケープ `%` クラッシュを解消し（全 40 引数ヘルプ AST 検証済み）、`verify-diff check-clean`・`recap-subworker` 不変条件テスト・`recap-worker` 分割ヘルパテストがすべて通過する | コマンド: `./scripts/altharness/alt-harness verify-diff check-clean && (cd recap-subworker && uv run --offline pytest -o addopts="" tests/unit/test_verify02_split_invariants.py tests/unit/test_pseudo_label_helpers.py tests/unit/test_async_jobs.py -q) && (cd recap-worker/recap-worker && cargo test --offline --lib pipeline::select -q && cargo test --offline --lib eval::metrics -q)` / 保存先: `docs/review/verify-02-differential-evidence.md` |

## 3. 未検証事項・残余リスク

- 再現可能な `fix` および `investigate` は **0 件**。残余の非ブロッキング `note` 3 件（`AUD-03`: `Manifest.Save` の新規 `0o700` ディレクトリ書込確認、`AUD-04`: `DistributingGateway.chat_stream` 未実装だが本番配線は `OllamaGateway` 直結、`REV-07`: `html_parser` の到達不能 `nilerr` フォールバックと `stream_article_tags_usecase` の単体テスト追加余地）を `docs/review/loop-04-adversarial-review-report.json` に記録済み。

## 4. 検証ゲートの再実行

```bash
./scripts/altharness/alt-harness loop-review validate docs/review/loop-04-adversarial-review-report.json --require-focus-all
./scripts/altharness/alt-harness verify-claims validate docs/review/loop-04-adversarial-review-evidence.md
./scripts/altharness/alt-harness loop-review check-coverage --strict-resolved
```
