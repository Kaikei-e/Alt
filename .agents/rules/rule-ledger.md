---
max_judgment_rules: 6
---

# Rule Ledger (CHECK-07)

This ledger pairs every operational and engineering rule in `AGENTS.md`, `CLAUDE.md`, and `.agents/rules/` with its concrete mechanical enforcement mechanism (or marks it as `判断` when human/agent judgment is still required).

`scripts/altharness/alt-harness selfcheck` (check **H11**) verifies that:
- Every non-`判断` row references at least one existing repository script/test path, `scripts/altharness/alt-harness <subcommand>`, or `ast-grep:<id>` rule under `scripts/lint/ast-grep/rules/`.
- Every `C-NN` corpus reference and `ADR-NNNNNN` reference in `由来` exists in the repository.
- The number of `判断` rows equals `max_judgment_rules` monotonically: adding new `判断` rules is blocked, and mechanizing a rule requires lowering `max_judgment_rules` in the same commit.

| ID | 規則 | 強制手段 | 由来 |
|---|---|---|---|
| R-01 | `.env`・`.kisage/*`・`secrets`・鍵ファイル・追跡済み migration を直接編集しない | `scripts/altharness/alt-harness guard-protected` | CLAUDE.md / AGENTS.md |
| R-02 | Knowledge projector はイベント時刻（`OccurredAt`）とペイロードのみを使い、`time.Now()` や `GetLatest*` を呼ばない | `scripts/altharness/alt-harness guard-event-time` | ADR-000447 / ADR-000468 |
| R-03 | 破壊的 Docker 操作や本番 Compose（`-p alt`）の再作成・停止は明示的承認なしに実行しない | `scripts/altharness/alt-harness guard-ops` | .agents/rules/deployment.md |
| R-04 | `CLAUDE.md` と `AGENTS.md` の共有ブロックを一致させ、文書サイズ上限・スキル解決・失敗の型台帳を維持する | `scripts/altharness/alt-harness selfcheck` | AGENTS.md / CLAUDE.md |
| R-05 | エージェント作業は `allowed_paths`・ファイル/関数上限・許可ルールで境界化し、3 項目ハンドオフを必須とする | `scripts/altharness/alt-harness packet` | .agents/rules/task-packet.md |
| R-06 | 本番コード編集前に binding ADR を確認し検証済み `docs/ADR/NNNNNN.md`（`proposed` → テスト通過後 `accepted`）を起票する（テストと `docs/` のみ免除）。ADR 参照時は `docdag` を使い、確定済み ADR は追記専用（置換は新 ADR） | `scripts/altharness/alt-harness preflight`, `scripts/altharness/alt-harness guard-adr-read`, `scripts/altharness/alt-harness adr-gate`, `scripts/lint/adr.sh` | CLAUDE.md / AGENTS.md / ADR-001032 / ADR-001035 |
| R-07 | Clean Architecture の内向き依存境界（`Handler → Usecase → Port → Gateway → Driver`）を遵守する | `.agents/skills/clean-architecture/scripts/check_layers.sh` | AGENTS.md / CLAUDE.md |
| R-08 | 破壊的 DB マイグレーションを無承認で導入しない | `scripts/altharness/alt-harness migration-gate` | scripts/lint/harness.sh |
| R-09 | ポストモーテムのアクションアイテムを未完了のまま持ち越さない | `scripts/altharness/alt-harness postmortem` | scripts/lint/harness.sh |
| R-10 | 追跡ファイルの 0 バイト破壊、未許可の 0 バイト未追跡ファイル、およびバイナリ成果物（ELF/Mach-O/PE）の残留を禁止する | `scripts/altharness/alt-harness hygiene` | C-19 / C-20 |
| R-11 | リント実行前にディスク空き容量下限を検証し、`LINT_POLICY`（`fix-existing` / `block-new`）を伝播する | `scripts/lint/run.sh` | C-19 |
| R-12 | ADR 根拠のない包括的 lint 抑制（`noqa` / `nolint` / `#nosec` / 設定 allowlist 拡張）を禁止する | `scripts/altharness/alt-harness suppressions` | C-22 |
| R-13 | 静的スキャナの 0 ファイル通過（サイレントパス）をカナリア監査で検知する | `scripts/altharness/alt-harness canary` | C-21 |
| R-14 | 各 Python サービスのランタイム・ターゲットバージョン整合性を維持し非互換構文の混入を防ぐ | `scripts/altharness/alt-harness pyver` | C-05 |
| R-15 | `deploy/desired-env.lkal` を検証し外部 API レート制限（`RATE_LIMIT_EXTERNAL_API_INTERVAL >= 5s`）を保証する | `scripts/kisage-env.py` | ADR-000982 |
| R-16 | 認証トークン直書きおよび昇格権限 CI トリガ（`pull_request_target` / `workflow_run`）を禁止する | `scripts/check-no-service-token.sh`, `scripts/altharness/alt-harness workflow` | ADR-000763 |
| R-17 | Protobuf 契約・破壊的変更・allowlist 同期および Pact CDC 契約を検証する | `scripts/lint/proto.sh`, `scripts/pact-check.sh` | ADR-000928 |
| R-18 | Compose include 構成および Go/Python 生成ヘルパの同期ズレを検出する | `scripts/lint/compose.sh`, `scripts/altharness/alt-harness sync-go`, `scripts/altharness/alt-harness pki` | AGENTS.md / CLAUDE.md |
| R-19 | Go の構造ガードレール（正規 HTTP サーバ、定数化された FileMode / BodyLimit、安全な proto/Connect nil ガード）を遵守する | `ast-grep:go-no-raw-http-server`, `ast-grep:go-no-octal-filemode-literal`, `ast-grep:go-no-inline-body-limit`, `ast-grep:go-no-manual-connect-msg-nil-guard`, `ast-grep:go-no-unguarded-response-proto-getter` | C-04 / C-11 / C-12 / C-15 |
| R-20 | Python・Rust の構造ガードレール（`finally` での手動 `aclose()` 禁止、`sys.modules` 直接破壊禁止、サイレント `Option<Regex>` 禁止）を遵守する | `ast-grep:python-no-manual-aclose-in-finally`, `ast-grep:python-no-direct-sys-modules-assign`, `ast-grep:rust-no-silent-option-regex` | C-07 / C-08 / C-17 |
| R-21 | 失敗コーパス（`C-01`..`C-22`）による回帰検証を維持し捕捉率低下を防ぐ | `scripts/altharness/alt-harness corpus` | C-01 / C-09 / C-21 / C-22 |
| R-22 | 生成コード（`Code generated ... DO NOT EDIT`）の手編集・未ピン留めジェネレータ・再生成ドリフトを禁止する | `scripts/altharness/alt-harness generated`, `scripts/altharness/alt-harness packet` | C-16 |
| R-23 | 構造 AST スクリーナで意図せぬ振る舞い変更候補を検出し、タスクハンドオフでの明示的承認を必須とする | `scripts/altharness/alt-harness screener`, `scripts/altharness/alt-harness packet` | C-01 / C-02 / C-03 / C-06 / C-10 / C-13 / C-14 |
| R-24 | インライン抑制数の単調減少ラチェット・期限切れ検査・PR 差分サマリを強制する | `scripts/altharness/alt-harness ratchet`, `scripts/altharness/alt-harness suppressions`, `scripts/altharness/alt-harness suppression-diff` | C-22 |
| R-25 | 全 46 ターゲットのカバレッジ台帳と 6 言語ファミリの end-to-end エントリポイントカナリアを検証する | `scripts/altharness/alt-harness entrypoint-canary`, `scripts/lint/canary_ledger.toml`, `scripts/altharness/alt-harness canary` | C-21 |
| R-26 | 実装前に失敗テストを書く（TDD: RED → GREEN → REFACTOR） | `scripts/altharness/alt-harness packet`, `scripts/altharness/alt-harness corpus` | AGENTS.md / CLAUDE.md / C-18 |
| R-27 | Go / Rust / TS 変更時は `--build` 付きでコンパイル済みサービスを再ビルドする | 判断（ビルド反映手順の確認） | AGENTS.md / CLAUDE.md |
| R-28 | 変更前に `<service>/CLAUDE.md` を確認し、修復 PR では正規契約（`docs/plan/knowledge-trail-core-concept.md`）を再読込する | 判断（着手時の文脈ロードで確認） | ADR-000940 |
| R-29 | 未配線 DI 依存のサイレントフォールバックを禁止し、起動ログとビジネス経路での panic を強制する | 判断（コードレビューと failure-pattern-review で確認） | ADR-000928 |
| R-30 | 必須設定欠落時は起動時に非ゼロ終了（fail-fast）し、無効化は明示設定とする | 判断（起動処理レビューで確認） | AGENTS.md / CLAUDE.md |
| R-31 | Stream コンシューマは永続化後に XACK し、XAUTOCLAIM 回収ループを実装する | 判断（コンシューマ実装レビューで確認） | AGENTS.md / CLAUDE.md |
| R-32 | ロジック改変を伴う一括 autofix（`golangci-lint --fix`, `cargo clippy --fix`, `ruff --unsafe-fixes`）を実行しない | `scripts/altharness/alt-harness guard-exec`, `.claude/hooks/deny-unsafe-commands.sh`, `scripts/altharness/alt-harness packet` | C-01 / C-02 / C-03 |
| R-33 | 検証・ビルドはフォアグラウンド同期実行とし、歴史的資産 `stopped-using-k8s/` は明示指示なしに変更しない | `scripts/altharness/alt-harness guard-exec`, `scripts/altharness/alt-harness guard-scope` | .agents/rules/task-packet.md |
| R-34 | `alt-backend/app` の 4 バイナリ（`backend` / `harvester` / `notifier` / `datahub`）責務分離を維持する | 判断（設計・変更レビューで確認） | ADR-000954 |
| R-35 | 長大関数分割の前後同値性を乱数差分試験で確認し、一時的な旧実装足場をリポジトリに残さない | `scripts/altharness/alt-harness verify-diff` | ADR-001037 |
| R-36 | Echo v5 4 サービス・プロキシ・コード生成器の実効設定を JSON ゴールデンで固定し、無断変更を差分検知する | `scripts/altharness/alt-harness verify-golden` | C-11 / C-12 / C-13 / C-14 / C-15 / ADR-001037 |
| R-37 | 本番 `-p alt` から完全隔離した実機検証スキル・Feature Map で SSE 終端・境界値・認証・転送ヘッダを実証する | `scripts/altharness/alt-harness verify-live`, `scripts/verify/verify-live.sh` | C-07 / C-14 / C-15 / ADR-001036 / ADR-001037 |
| R-38 | 不安定テストはシャッフルシード付きで再現・特定してから修正し、回帰テストで固定する | `scripts/altharness/alt-harness flaky-hunt`, `scripts/verify/flaky-hunt.sh` | ADR-001037 |
| R-39 | 実装報告の挙動主張ごとに再実行可能なコマンドと出力証拠の保存先を必須とし、裏付けのない包括主張を拒否する | `scripts/altharness/alt-harness verify-claims`, `scripts/agent-harness/templates/implementation-report.md.tmpl` | ADR-001037 |
| R-40 | ホスト受入検証は実 `git diff`・AST 関数計数・検証コマンド再実行・乱数順テストと事前実測予算（`--turn-limit-s`）を強制する | `scripts/altharness/alt-harness packet`, `scripts/altharness/alt-harness host-final`, `scripts/verify/host-final-check.sh` | ADR-001039 |
| R-41 | ブランチ単位の判断ログ（`decisions.tsv`）の 6 列スキーマ・1 行目方針/完了定義・ポインタ証拠・信頼に基づく並列数変更記録を維持する | `scripts/altharness/alt-harness harness-run` | ADR-001039 |
| R-42 | push 前の敵対的レビューは異系統モデル・4 分類・非自動適用・`fix` の再現証明・`safe` の `blast-radius` 実行証明と重点 7 項目網羅を強制する | `scripts/altharness/alt-harness loop-review` | C-01 / C-04 / C-06 / C-07 / C-11 / ADR-001039 |
| R-43 | パケット定型や規則の改訂は中立ディレクトリ隔離・決定論的採点・トランスクリプト読込検証と前後 3 試行以上比較で評価し判断ログへ記録する | `scripts/altharness/alt-harness loop-eval` | C-01 / C-06 / C-07 / C-11 / C-15 / C-16 / C-18 / ADR-001039 |
| R-44 | 区切りごとの `/correct` で 2 回以上再発した失敗クラスを抽出し、コーパス登録と `ARCH→CHECK→VERIFY→LEDGER` 上位段却下理由の記録を強制する | `scripts/altharness/alt-harness loop-correct` | ADR-001039 |
| R-45 | Tauto 形式仕様のゼロ公理・ゼロ hole・カナリア監査および生成適合ベクタの手編集・ドリフト防止を強制する | `scripts/lint/tauto.sh`, `scripts/altharness/alt-harness canary`, `scripts/altharness/alt-harness generated` | ADR-001029 |
| R-46 | 変更された Go / Python / TypeScript・Svelte ファイルの静的リント違反を残したままエージェント作業を終了しない | `scripts/altharness/alt-harness lint-changed`, `scripts/altharness/alt-harness agy-hook` | ADR-001045 |
