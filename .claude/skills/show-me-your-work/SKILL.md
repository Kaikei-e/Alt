---
name: show-me-your-work
description: >
  Alt の LOOP-03（判断ログ `decisions.tsv`）および LOOP-06（信頼に基づく並列数制御）の運用スキル。
  ブランチ単位の `.harness/runs/<branch>/decisions.tsv` に 6 列スキーマ・1 行目の方針と完了定義・
  追記専用ルール・具体的ポインタ証拠・並列数変更の理由つき記録を機械検証しながら蓄積する。
  「判断ログを残して」「decisions.tsv を検証して」「show-me-your-work」「並列数を見直して」
  「セッションを引き継いで」と言われたとき、またはタスクパケット運用・レビュー・並列実行の区切りで起動する。
allowed-tools:
  - "Bash(./scripts/altharness/alt-harness harness-run *)"
  - Read
  - Grep
  - Glob
---

# Show Me Your Work (LOOP-03 判断ログ & LOOP-06 並列数制御スキル)

長時間セッションやセッション切替をまたいで方針・完了定義・判断根拠・並列数変更を失わないため、
ブランチ単位の `.harness/runs/<branch>/decisions.tsv` に**1 判断 1 行・追記専用・具体的ポインタ証拠つき**で記録し、
機械検証（`harness-run validate`）と並列数ラチェット（`harness-run parallelism`）を回すスキルである。

## 1. 永続ディレクトリ構造（`.harness/runs/<branch>/`）

すべての実行状態はリポジトリ配下の `.harness/runs/<sanitized-branch>/`（`.gitignore` 対象）に永続化される。

```text
.harness/runs/<sanitized-branch>/
├── decisions.tsv         # LOOP-03/06 追記専用の 6 列判断ログ（正本）
├── parallelism.json      # LOOP-06 直近の並列数評価・適用スナップショット
├── active-packet.json    # 現在アクティブな TaskPacket のスコープ制約状態
├── packets/              # 投入済み TaskPacket スナップショット（<packet_id>.md）
├── handoffs/             # 受領済み TaskHandoff 記録（<packet_id>.md|.json）
├── verify/               # LOOP-01 ホスト再実行の証拠 JSON（<packet_id>.json）
├── evidence/             # 追加検証ログ・差分・レビュー成果物
└── subagents.jsonl       # SubagentStop フック記録
```

### セッション再開時の必須手順

新しいセッションやコンテキスト圧縮後に作業を再開する際は、最初に以下を実行して現行の方針・完了定義・直近判断・並列数を復元する。

```bash
./scripts/altharness/alt-harness harness-run status
./scripts/altharness/alt-harness harness-run validate
```

## 2. `decisions.tsv` の 6 列スキーマと不変条件（LOOP-03）

### 固定ヘッダ（厳密一致）

```tsv
ts	phase	decision	why	evidence	result
```

| 列 | 内容 | 機械検証ルール |
|---|---|---|
| `ts` | 記録時刻 | RFC3339 / ISO8601 UTC（例: `2026-11-28T00:00:00Z`） |
| `phase` | 工程 | 非空・単一行（例: `init`, `dispatch`, `measure`, `handoff`, `verify`, `review`, `parallelism`, `correct`） |
| `decision` | 下した判断 | 非空・単一行（タブ・改行禁止）。先頭の式インジェクション文字（`=`, `+`, `@`）は `'` 接頭で自動無害化 |
| `why` | 判断理由 | 非空・単一行（タブ・改行禁止）。先頭の `=`, `+`, `@` も同様に自動無害化 |
| `evidence` | 具体的ポインタ証拠 | **文章やプレースホルダ禁止**（後述のポインタ規則を満たすこと） |
| `result` | 結果語彙 | `adopted` / `reverted` / `mismatch` / `cancelled` / `info` のいずれか |

### 4 つの非妥協不変条件

1. **Row 1 方針＋完了定義（必須）**:
   - データ 1 行目（ファイル 2 行目）は必ず `phase == "init"` であり、`decision` に有効な `policy=fix-existing` または `policy=block-new` と、空でない `completion=<完了定義>` の両方を含むこと。
   - 必ず `harness-run init` で初期化し、手書きで 1 行目を省略しない。
2. **追記専用（Append-Only）**:
   - 既存行の編集・削除・上書きは禁止。方針変更ややり直しも必ず新しい行として追記する。
   - スプレッドシート等で開いた際の式インジェクションを防ぐため、`AppendDecisionToFile` は `decision`・`why` 先頭の `=`, `+`, `@` に `'` を付与して無害化する。
3. **具体的ポインタ証拠（Concrete Pointer Only）**:
   - 受理される形式: リポジトリ相対パス（`docs/ADR/001039.md`, `.harness/runs/.../decisions.tsv`）、`commit:<sha>`、`cmd:<command>`、`#<seq>`、`ADR-NNNNNN`、`PM-YYYY-NNN`、`C-NN`、`https://...`、`file://...`。
   - 拒否される例: `TBD`, `N/A`, `ok`, `done`, `looks good`, `なし`, `-`, 空文字。
4. **機械検証の通過**:
   - 区切りごとに `harness-run validate` を実行し、終了コード `0` を確認する（不整合は終了コード `1`、ファイル欠損や引数誤りは `2`）。

## 3. コマンド一覧（LOOP-03 / LOOP-06）

```bash
# 1. ブランチ実行ディレクトリの初期化（Row 1 に policy= と completion= を記録）
./scripts/altharness/alt-harness harness-run init \
  --policy block-new \
  --goal "対象モジュールの回帰修正" \
  --completion "./scripts/lint/run.sh <target> && go test -race ./... exits 0"

# 2. 判断を 1 行追記（追記専用・ポインタ証拠と語彙を検証）
./scripts/altharness/alt-harness harness-run log \
  --phase verify \
  --decision "host rerun green" \
  --why "LOOP-01 verify-handoff passed without diff drift" \
  --evidence ".harness/runs/<branch>/verify/<packet_id>.json" \
  --result info

# 3. decisions.tsv の構造・Row 1・時刻・証拠・語彙を機械検証
./scripts/altharness/alt-harness harness-run validate
./scripts/altharness/alt-harness harness-run validate --decisions-file <path> --format json

# 4. 現行ブランチの実行状態と推奨並列数を確認
./scripts/altharness/alt-harness harness-run status
./scripts/altharness/alt-harness harness-run status --format json

# 5. TaskPacket の有効化（検証・スコープ固定・dispatch 行の追記）
./scripts/altharness/alt-harness harness-run activate-packet <packet.md>

# 6. TaskHandoff の結果確定（active 解除・終端結果 adopted|reverted|mismatch|cancelled を追記）
./scripts/altharness/alt-harness harness-run record-handoff <handoff.md> \
  --result adopted \
  --why "LOOP-01 verify-handoff passed"

# 7. LOOP-06 信頼に基づく並列数の評価と理由つき記録
./scripts/altharness/alt-harness harness-run parallelism --format json
./scripts/altharness/alt-harness harness-run parallelism --apply
./scripts/altharness/alt-harness harness-run parallelism --apply --record-hold
./scripts/altharness/alt-harness harness-run parallelism --current 4 --apply
```

## 4. 信頼に基づく並列数制御（LOOP-06）

並列数は直近 10 件の終端結果（`adopted`, `reverted`, `mismatch`, `cancelled`。`info` 行は集計対象外）から決定論的に算出する。

| 条件（直近 10 件ウィンドウ） | 変更 | 範囲 |
|---|---|---|
| 終端結果 0 件 | 変更なし（据え置き） | 既定ベース `2` |
| `reverted >= 3` または `mismatch >= 2` | `-2`（即時縮小） | 下限 `1` |
| ウィンドウ 10 件到達かつ `adopted > 0`, `reverted <= 1`, `mismatch == 0` | `+2`（段階拡大） | 上限 `8` |
| 上記以外（ウォームアップ・安定帯） | 変更なし（据え置き） | 現行値維持 |

### 現行並列数の解決順と永続化

1. `--current N` が明示されていればその値（`1..8`）を採用する。
2. 省略時は `decisions.tsv` の直近 `phase == "parallelism"` 行（`parallelism <old> -> <new>` または `parallelism=<new>`）から前回の確定並列数を自動引き継ぎする。
3. 未記録時は `--base-parallelism`（既定 `2`）を用いる。
4. `--apply` 指定時に並列数が変化した場合（または `--record-hold` 指定時）、`decisions.tsv` へ `phase=parallelism`・`decision="parallelism <old> -> <new>"`・`why="<集計と理由>"`・`evidence="<decisions.tsv 相対パス>"`・`result=info` を追記し、同時に `.harness/runs/<branch>/parallelism.json` へ `{current, recommended, delta, window_size, counts, reason, updated_at, changed}` を保存する。

## Prior-Art Attribution

本スキルと `decisions.tsv` の検証・並列数追跡機構は、Alt 自身の運用記録（`docs/ADR/001028.md` 純 Go 標準ライブラリ単一バイナリ方針、`docs/ADR/001039.md` LOOP-01〜07 統合、`docs/review/lint-rollout-harness-learnings-2026-11-21.md` のセッション越境・黙殺失敗対策）を第一の根拠として設計しつつ、6 列判断ログ・追記専用規律・式インジェクション衛生の運用様式について `cursor/plugins` の `pstack`（`show-me-your-work`, MIT License、一次資料: <https://github.com/cursor/plugins/blob/main/pstack/docs/guide/README.md>）を参照したクリーンルーム実装である。
