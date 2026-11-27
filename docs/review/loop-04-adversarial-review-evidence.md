---
report_version: "1.0"
task_id: "LOOP04-2026-11-28"
target: "alt-backend/app, altctl, auth-hub, pre-processor/app, search-indexer/app, rag-orchestrator, news-creator/app, recap-subworker, recap-worker"
base_commit: "eb978e1377f51cd4e5d2c741a703d882f131e351"
head_commit: "f3ab6fba1d70b34b64c3e80adce6b7cbc06e5649"
---

# LOOP-04 敵対的レビュー実測証拠台帳: LOOP04-2026-11-28

## 1. 変更概要

- 対象サービス / モジュール: `alt-backend/app`, `altctl`, `auth-hub`, `pre-processor/app`, `search-indexer/app`, `rag-orchestrator`, `news-creator/app`, `recap-subworker`, `recap-worker`
- 監査モデル体制: 実装・比較側 `gemini-4-neon-xhigh` に対し、別モデル系統 `gemini-4-carbon-high`（read-only 敵対的レビュアー、`auto_applied: false`）が LOOP-04 重点 7 項目（`F-01`〜`F-07`）を実コード・実テスト駆動（`/interrogate` + `/blast-radius`）で検証した。
- 判定集計: `safe` 5 件（`F-01`, `F-02`, `F-03`, `F-05`, `F-07`）、`deferred` 2 件（`F-04`, `F-06` — 再現確認済みバグ 3 件は非自動適用原則に従い独立した修正 PR 対象として記録）。
- 指摘 4 分類（全 9 件）: `fix` 3 件（`REV-05`, `REV-06`, `REV-08`）、`investigate` 2 件（`REV-03`, `REV-04`）、`note` 3 件（`REV-01`, `REV-07`, `REV-09`）、`dismiss` 1 件（`REV-02`）。

## 2. 主張と証拠（VERIFY-06 必須）

| 主張 | 証拠（コマンドと出力の保存先） |
| --- | --- |
| `F-01` (`safe`): `errorlint` 書き換え 4 箇所（`C-01` `>= 500` 条件・`C-02` `!errors.As` 否定）の意味論が維持されている | コマンド: `go -C alt-backend/app test ./middleware/... ./connect/errorhandler/... ./orchestrator/rest/rest_feeds/summarization/... && go -C altctl test ./cmd/...` / 保存先: `docs/review/loop-04-adversarial-review-report.json` |
| `F-02` (`safe`): `pre-processor` / `search-indexer` / `alt-backend` の全単項 Connect-RPC クライアントが `connectguard.NewUnaryNilResponseInterceptor()` を配線し `C-04` ast-grep 0 件 | コマンド: `ast-grep scan -c scripts/lint/ast-grep/sgconfig.yml pre-processor/app search-indexer/app alt-backend/app && go -C search-indexer/app test ./driver/backend_api/... ./internal/connectguard/...` / 保存先: `docs/review/loop-04-adversarial-review-report.json` |
| `F-03` (`safe`): `auth-hub` の設定ファイル読込は `os.OpenRoot` で `..`/symlink を拒否し、`httpserver.New` で `ReadHeaderTimeout=10s`（無断 `IdleTimeout` なし、`C-11`）を維持 | コマンド: `go -C auth-hub test -race ./...` / 保存先: `auth-hub/cmd/auth-hub/testdata/effective_config.golden.json` |
| `F-04` (`deferred` / `REV-05`, `REV-06` `fix`): 既存の字句 `..` テストは通過するが、`altctl` の `resolveUnderBase` と `CreateEnvFile` に symlink 脱出バグ 2 件を `exit_code=1` で再現 | コマンド: `go -C altctl test -race ./...` / 保存先: `docs/review/loop-04-adversarial-review-report.json` |
| `F-05` (`safe`): 本番コードの `//nolint:nilerr` 全 10 箇所（`alt-backend/app` 9、`rag-orchestrator` 1）が構造化ログ出力＋明示的なドメイン縮退値を返し対象テスト通過 | コマンド: `go -C alt-backend/app test ./utils/html_parser/... ./shared/driver/redis_slot_store/... && go -C rag-orchestrator test ./internal/backfill/...` / 保存先: `docs/review/loop-04-adversarial-review-report.json` |
| `F-06` (`deferred` / `REV-08` `fix`): `HybridPrioritySemaphore` (`C-06`)・`sse_stream.py` (`C-07`)・`QueueFullError` 424 テストは通過するが、`OllamaGateway.chat_stream` の `aclosing` 欠落による上流ストリーム未閉鎖を `exit_code=1` で再現 | コマンド: `cd news-creator/app && uv run --offline pytest tests/gateway/ tests/handler/test_sse_stream.py tests/handler/test_summarize_handler.py -q` / 保存先: `docs/review/loop-04-adversarial-review-report.json` |
| `F-07` (`safe`): `verify-diff check-clean`・`recap-subworker` 21 不変条件テスト・`recap-worker` 分割ヘルパテストがすべて通過し分割前後同値性を維持 | コマンド: `./scripts/altharness/alt-harness verify-diff check-clean` / 保存先: `docs/review/verify-02-differential-evidence.md` |

## 3. 未検証事項・残余リスク

- `REV-05`（`altctl/internal/migrate/manifest.go:128` の `resolveUnderBase` symlink 脱出）、`REV-06`（`altctl/internal/setup/envfile.go:36` の `CreateEnvFile` dangling/live symlink 追従と `--force` 時 `Chmod(0o600)` 欠落）、`REV-08`（`news-creator/app/news_creator/gateway/ollama_gateway.py:989` の `chat_stream` における `async with aclosing(...)` 欠落）は `auto_applied: false` 原則により本ハーネス PR では自動変更せず、再現コマンド付きで `docs/review/loop-04-adversarial-review-report.json` および `decisions.tsv` へ記録した。個別の修正 PR で是正が必要。
- `REV-03`（`search-indexer/app/driver/backend_api/client.go:173,248` の nil タイムスタンプ `.AsTime()`）と `REV-04`（`auth-hub/config/config.go:316` の `<KEY>_FILE` エラー時の既定値フォールバック）は `investigate` として追跡対象。

## 4. 検証ゲートの再実行

```bash
./scripts/altharness/alt-harness loop-review validate docs/review/loop-04-adversarial-review-report.json --require-focus-all
./scripts/altharness/alt-harness verify-claims validate docs/review/loop-04-adversarial-review-evidence.md
./scripts/altharness/alt-harness loop-review check-coverage
```
