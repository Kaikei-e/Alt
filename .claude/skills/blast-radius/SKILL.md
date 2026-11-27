---
name: blast-radius
description: >
  Alt の影響範囲・安全前提実証スキル。差分の外側にある耐荷重な安全前提（呼び出し元・契約・nil 境界・
  エラー伝播・権限・並行資源・前後同値性）を洗い出し、grep や文章の言い切りだけで「安全」と結論づけず、
  実コード・実テスト・隔離実機プローブの実行（exit_code == 0）によって安全性を証明し、
  LOOP-04 レポートの `blast_radius_proof` へ統合する。
  「/blast-radius」「影響範囲を実証して」「安全性をコードで証明して」「blast radius」「壊れていない根拠」
  と言われたとき、または `/interrogate` で重点項目を `safe` 判定する前に起動する。
allowed-tools:
  - "Bash(./scripts/altharness/alt-harness loop-review *)"
  - "Bash(./scripts/altharness/alt-harness verify-diff *)"
  - "Bash(./scripts/altharness/alt-harness verify-golden *)"
  - "Bash(./scripts/verify/verify-live.sh *)"
  - "Bash(git diff --stat *)"
  - "Bash(go * test *)"
  - "Bash(uv run pytest *)"
  - "Bash(cargo test *)"
  - "Bash(ast-grep *)"
  - Read
  - Grep
  - Glob
---

# Blast Radius (Prove Safety Outside the Diff by Running Real Code)

変更差分の「中」だけを読んで安全と言い切ることはできない。真の回帰は**差分の外側にある呼び出し元・契約・境界値・エラーハンドラ・起動設定**が暗黙に依存していた前提が崩れたときに起きる。

このスキルは、差分の外にある耐荷重な安全前提（load-bearing safety assumptions）を列挙し、
**`grep` や文章説明だけで済ませず、実コード・実テストを実行して証明する**ための手順を定義する。

> **Non-Negotiable Rule**:
> 「コードを読んだので安全」「`rg` で呼び出し元が 0 件だったので問題ない」といった**文章だけの安全主張は禁止**する。
> すべての `safe` 判定には、実コマンドの実行結果（`exit_code == 0`）と具体ポインタ証拠を備えた `blast_radius_proof` を添付しなければならない（未証明の `safe` 判定は `alt-harness loop-review validate` が `UNPROVEN_SAFE_CLAIM` で機械的に拒否する）。

## 1. Identify Load-Bearing Assumptions Outside the Diff

対象変更のファイル一覧を確認し（`git diff --stat <base>` または `alt-harness loop-review brief --base <base>`）、差分の外側で壊れうる前提を具体化する:

| Change Category | Load-Bearing Assumption to Verify Outside the Diff |
|---|---|
| 条件式・型アサーション書き換え（`F-01`, `C-01`/`C-02`） | `&& he.Code >= 500` や `!` 否定条件が変わらず、4xx/5xx 分岐やミドルウェアのレート制限挙動が呼び出し元テストで維持されていること |
| Proto / Connect-RPC nil ガード（`F-02`, `C-04`） | 上流 RPC が `nil` 応答を返したときにゼロ値でサイレント継続せず、明示エラーで fail-fast すること |
| 設定・シークレット読み込み・ファイル権限（`F-03`, `F-04`, `C-11`） | 必須設定欠落で起動時 fail-fast し、`..` / symlink 脱出が拒否され、機密ファイル権限 `0600` が保たれること |
| `nilerr` 縮退箇所（`F-05`, `C-17`） | `err != nil` で `nil` を返す経路が意図的フォールバックであり、上位処理の整合性や障害検知を破壊しないこと |
| 非同期ゲートウェイ・セマフォ・SSE（`F-06`, `C-06`/`C-07`） | `QueueFullError` 等の制御例外が上位へ正しく伝播し、`aclose()` 例外時でも SSE がハングせず終端すること |
| 長大関数分割（`F-07`, `VERIFY-02`） | 分割前後の入出力同値性が特性テストで維持され、一時的な旧実装足場がリポジトリに残っていないこと |

## 2. Prove Each Assumption by Running Real Code

各前提ごとに、**実際にコードを実行するコマンド**を選んで走らせる（モック過剰な単体テストで不十分な境界は隔離ライブ検証 `./scripts/verify/verify-live.sh` やゴールデン検証 `verify-golden` を併用する）:

```bash
# 例 1: Go ミドルウェア / エラーハンドラの条件式・レース検査 (F-01)
go -C alt-backend/app test -race -count=1 ./middleware/... ./connect/errorhandler/...

# 例 2: Connect-RPC / Proto nil ガード構造検査 + テスト (F-02)
ast-grep scan --config scripts/lint/ast-grep/sgconfig.yml pre-processor/app search-indexer/app alt-backend/app
go -C pre-processor/app test -race -count=1 ./...

# 例 3: auth-hub 設定・権限・fail-fast (F-03) / altctl パス封じ込め (F-04)
go -C auth-hub test -race -count=1 ./...
go -C altctl test -race -count=1 ./...

# 例 4: news-creator ゲートウェイ・SSE 終端 (F-06)
cd news-creator/app && uv run pytest tests/gateway/ tests/handler/ -q
./scripts/verify/verify-live.sh run --task blast-radius-f06 --probe sse

# 例 5: 長大関数分割の一時足場残留チェック (F-07)
./scripts/altharness/alt-harness verify-diff check-clean
```

### 実行時の安全・隔離規約

- 本番 Docker Compose（`-p alt`）に対する操作・`docker system prune` 等は一切行わない（`guard-ops` 準拠）。
- ライブ HTTP / SSE / プロキシ経路を検証する場合は必ず隔離ループバック検証（[`verify-alt`](../verify-alt/SKILL.md)）を使う。
- 検証のために一時的なテスト・デバッグファイルを残さない（作業後は `git status` と `alt-harness hygiene` でクリーン状態を確認する）。

## 3. Attach `blast_radius_proof` to LOOP-04 Reports

実行して `exit_code == 0` を直接観測できた前提だけを `focus_verdicts[].blast_radius_proof` に記録する:

```json
{
  "focus_id": "F-03",
  "title": "auth-hub の設定ファイル読み込み",
  "status": "safe",
  "summary": "Missing required config fails fast at startup and secret files enforce strict mode",
  "evidence": "auth-hub/config/config_test.go",
  "blast_radius_proof": {
    "assumption": "auth-hub config loader fails fast on missing/unreadable secrets and preserves timeout defaults",
    "command": "go -C auth-hub test -race -count=1 ./config/...",
    "exit_code": 0,
    "evidence": "auth-hub/config/config_test.go"
  }
}
```

### 実行できない・失敗した場合の扱い

1. **実行して失敗した（`exit_code != 0`）**: 安全ではない。`safe` にしてはならない。失敗ログを `.harness/evidence/loop-04/<id>.log` 等に保存し、[`interrogate`](../interrogate/SKILL.md) の `findings` に `bucket: "fix"`＋`repro` として起票する。
2. **環境制約で実行できなかった**: 憶測で `safe` と書かない。`status: "deferred"` として未実行の理由と必要な検証コマンドを `summary` に明記するか、`bucket: "investigate"` として記録する。

## 4. Machine Verification

```bash
./scripts/altharness/alt-harness loop-review validate report.json --require-focus-all --format text
```

`validate` はすべての `status == "safe"` 項目について `blast_radius_proof` の 4 項目（非プレースホルダ `assumption`、非プレースホルダ `command`、明示的な `exit_code == 0`、具体ポインタ `evidence`）を fail-closed で検証し、1 つでも欠けていれば `UNPROVEN_SAFE_CLAIM` で拒否する。

## Prior-Art Attribution

本スキルと `blast_radius_proof` 検証契約は、Alt 自身の失敗コーパス（`CORPUS-01`）・層 3 動的検証（[[001037]]）・LOOP-04 敵対的レビュー基盤（[[001039]]）の不変条件を起点に設計しつつ、差分外の耐荷重前提を文章ではなく実コード実行で実証するアプローチについて `cursor/plugins` の `pstack`（`/blast-radius`、MIT License、一次資料: <https://github.com/cursor/plugins/blob/main/pstack/docs/guide/README.md>）を参照したクリーンルーム実装である。
