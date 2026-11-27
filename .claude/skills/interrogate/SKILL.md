---
name: interrogate
description: >
  Alt の LOOP-04 敵対的マルチモデルレビュー用スキル。無方向な全差分ダンプを避け、
  `alt-harness loop-review brief|validate|record|check-coverage` を用いて重点 7 項目（F-01〜F-07）・
  CORPUS-01・CHECK-01 候補を異系統モデルで「壊す役」として監査し、4 分類（fix / investigate / note / dismiss）・
  `fix` への再現コマンド/テスト証明・`safe` への blast-radius 実行証明・`auto_applied: false`・
  判断ログ（decisions.tsv）への 1 指摘 1 行記録を強制する。
  「/interrogate」「敵対的レビュー」「壊す役でレビュー」「LOOP-04」「異系統モデルで監査して」と言われたとき、
  またはマージ・大きなリファクタリングの最終確認時に起動する。
allowed-tools:
  - "Bash(./scripts/altharness/alt-harness loop-review *)"
  - "Bash(./scripts/altharness/alt-harness screener *)"
  - "Bash(git diff --stat *)"
  - "Bash(git diff --name-status *)"
  - "Bash(go * test *)"
  - "Bash(uv run pytest *)"
  - "Bash(cargo test *)"
  - "Bash(ast-grep *)"
  - Read
  - Grep
  - Glob
---

# Interrogate (LOOP-04 Adversarial Multi-Model Review)

実装者とは**別のモデル系統（cross-family）**に「壊す役」を渡し、無方向な全差分ダンプではなく
**重点対象・過去の失敗コーパス（`CORPUS-01`）・構造変化候補（`CHECK-01`）**に絞って欠陥を掘り起こし、
証拠つきでトリアージ・検証・判断ログ記録まで閉じるスキルである。

> **Core Invariants (Fail-Closed by `alt-harness loop-review validate`)**:
> 1. **No Raw Whole-Diff Dump**: レビュー依頼は必ず `alt-harness loop-review brief` で生成した焦点化パケットから始める。
> 2. **Distinct Model Family**: `reviewers` に `author_model` とは異なるモデル系統を最低 1 つ含める（例: 実装 `gemini-4-neon-*` → レビュー `gemini-4-carbon-*` / `claude-opus-*` / `gpt-*`。同系統 `gemini-4-neon-high` vs `gemini-4-neon-xhigh` は `SINGLE_MODEL_FAMILY_REVIEW` で拒否）。
> 3. **No Auto-Apply**: `auto_applied` は常に `false`（`AUTO_APPLY_FORBIDDEN`）。レビュー中にコードを無断修正しない。
> 4. **Repro Proof for `fix`**: `bucket: "fix"` の指摘には再現コマンドまたは失敗テスト（`exit_code != 0` と具体証拠）を必須とする（欠落時は `FIX_MISSING_REPRO_PROOF`）。
> 5. **Runnable Proof for `safe`**: `status: "safe"` の重点判定には `/blast-radius` で得た実コード実行証明 `blast_radius_proof`（`exit_code: 0`）を必須とする（文章だけの安全宣言は `UNPROVEN_SAFE_CLAIM` で拒否）。

## 1. Canonical 7 Focus Areas (LOOP-04)

| ID | Focus Area | Primary Inspection Targets | CORPUS-01 |
|---|---|---|---|
| `F-01` | `errorlint が書き換えた条件式` | `alt-backend/app/middleware/dos_protection_middleware.go`, `alt-backend/app/connect/errorhandler/error_handler.go`, `alt-backend/app/orchestrator/rest/rest_feeds/summarization/helpers.go`, `altctl/internal/config/config.go` | `C-01`, `C-02` |
| `F-02` | `protogetter の nil ガード` | `pre-processor/app`, `search-indexer/app`, `alt-backend/app` | `C-04` |
| `F-03` | `auth-hub の設定ファイル読み込み` | `auth-hub/config/config.go`, `auth-hub/cmd/auth-hub/main.go` | `C-11` |
| `F-04` | `altctl のパス封じ込め` | `altctl/internal/` | `C-11` |
| `F-05` | `nilerr の縮退箇所` | `alt-backend/app`, `pre-processor/app`, `rag-orchestrator`, `search-indexer/app` | `C-17` |
| `F-06` | `news-creator のゲートウェイ` | `news-creator/app/news_creator/gateway/`, `news-creator/app/news_creator/handler/sse_stream.py` | `C-06`, `C-07` |
| `F-07` | `長大関数の分割` | `docs/review/verify-02-differential-evidence.md`, `recap-subworker/recap_subworker/`, `recap-worker/recap-worker/` | `C-06` |

カスタム対象を追加・差し替えする場合は `--focus-file <focus.json>` を使う。ただし LOOP-04 完了判定（`--require-focus-all` / `check-coverage`）には上記 `F-01`〜`F-07` 全 7 件が必要。

## 2. Four-Bucket Triage Rules

すべての指摘は必ず次の 4 バケットのいずれか 1 つに分類し、`file:line` 等の具体ポインタ証拠と理由を添える:

| Bucket | Meaning | Mandatory Proof |
|---|---|---|
| `fix` | 修正すべき確定欠陥 | **`repro` 必須**: 実行可能な `command` または `test_name`、`exit_code != 0`（バグ再現 / テスト失敗）、具体的な `proof_evidence` ポインタ。推測や机上の懸念は `fix` に入れてはならない |
| `investigate` | 追加計測・追跡が必要な具体的疑念 | 対象ファイル・行と確認すべき仮説を明記 |
| `note` | 設計上の注意点・意図的挙動の確認メモ | 将来の保守者向けに不変条件と根拠ファイルを示す |
| `dismiss` | 検証の結果、問題なし・誤検知と棄却した仮説 | 棄却根拠となる実装・テストの具体ポインタを示す |

## 3. End-to-End Workflow

### Step 1: Generate a Focused Brief (Never Raw Whole-Diff)

```bash
./scripts/altharness/alt-harness loop-review brief \
  --base main \
  --format markdown \
  --out .harness/runs/<branch>/loop-04-brief.md
```

- 機械可読 JSON が必要な場合は `--format json` を指定する。
- `brief` は重点 7 項目・`CORPUS-01` 要約・インプロセス `CHECK-01` スクリーナ候補・`git diff --stat` のみを収録し、生の差分ハンク全体は出力しない。

### Step 2: Run Cross-Family Adversarial Review (Read-Only)

- 実装モデル（`author_model`）とは別系統のレビューモデル（例: Carbon High / Claude Opus 等）へ `brief` を渡し、読み取り専用で調査させる。
- 重点項目を `status: "safe"` と判定する場合は、必ず [`blast-radius`](../blast-radius/SKILL.md) の手順に従って**実コードまたは実テストを実行**し、`blast_radius_proof`（`assumption`, `command`, `exit_code: 0`, `evidence`）を記録する。
- 再現失敗した欠陥だけを `bucket: "fix"` とし、未検証の疑念は `investigate` に留める。
- `auto_applied` は必ず `false` とする（修正はレビュー完了後に別パケットで実施する）。

### Step 3: Report JSON Schema

```json
{
  "review_id": "LOOP04-2026-11-28",
  "base_commit": "<sha>",
  "head_commit": "<sha>",
  "author_model": "gemini-4-neon-xhigh",
  "reviewers": ["gemini-4-carbon-high"],
  "auto_applied": false,
  "focus_verdicts": [
    {
      "focus_id": "F-01",
      "title": "errorlint が書き換えた条件式",
      "status": "safe",
      "summary": "All 3 errorlint sites preserve && he.Code >= 500 and negation semantics",
      "evidence": "alt-backend/app/middleware/dos_protection_middleware_test.go",
      "blast_radius_proof": {
        "assumption": "HTTPError status >= 500 predicate preserved across all errorlint sites",
        "command": "go -C alt-backend/app test -race ./middleware/... ./connect/errorhandler/...",
        "exit_code": 0,
        "evidence": "alt-backend/app/middleware/dos_protection_middleware_test.go"
      }
    }
  ],
  "findings": [
    {
      "id": "REV-01",
      "focus_id": "F-06",
      "reviewer_model": "gemini-4-carbon-high",
      "bucket": "fix",
      "summary": "QueueFullError masked by broad exception catch",
      "rationale": "Caller receives generic 500 instead of backpressure signal",
      "evidence": "news-creator/app/news_creator/gateway/ollama_gateway.py:118",
      "repro": {
        "command": "cd news-creator/app && uv run pytest tests/gateway/test_ollama_gateway.py -k queue_full",
        "test_name": "test_queue_full_propagates",
        "exit_code": 1,
        "proof_evidence": ".harness/evidence/loop-04/rev-01.log"
      }
    }
  ]
}
```

### Step 4: Validate, Record to `decisions.tsv`, and Verify 7/7 Coverage

```bash
# 1. Fail-closed validation (enforces cross-family, auto_applied=false, repro proofs, blast_radius_proof, F-01..F-07)
./scripts/altharness/alt-harness loop-review validate report.json --require-focus-all --format text

# 2. Save validated copy under .harness/runs/<branch>/reviews/<review_id>.json
#    and append 1 row/finding (phase=interrogate) + 1 row/focus verdict (phase=loop-04) to decisions.tsv
./scripts/altharness/alt-harness loop-review record report.json --require-focus-all --format text

# 3. Verify all 7 canonical focus areas (F-01..F-07) are recorded in decisions.tsv
./scripts/altharness/alt-harness loop-review check-coverage --format text
```

- `validate` が 1 件でもエラー（`INVALID_REVIEW_ID`, `AUTO_APPLY_FORBIDDEN`, `SINGLE_MODEL_FAMILY_REVIEW`, `FIX_MISSING_REPRO_PROOF`, `UNPROVEN_SAFE_CLAIM`, `MISSING_FOCUS_ITEM`, `INVALID_EVIDENCE`, 重複 ID 等）を返した場合、`record` は一切ファイルを書き込まず終了コード 1 で停止する。

## Prior-Art Attribution

本スキルと `alt-harness loop-review` の検証契約は、Alt 自身の lint ロールアウト障害記録（`docs/review/lint-rollout-harness-learnings-2026-11-21.md`）、失敗コーパス（`CORPUS-01`）、および [[001039]] の純 Go 標準ライブラリ単一バイナリ不変条件を第一の根拠として設計しつつ、異系統モデルによる敵対的レビュー・4 分類トリアージ・非自動適用・再現証明要求のワークフロー設計について `cursor/plugins` の `pstack`（`/interrogate`、MIT License、一次資料: <https://github.com/cursor/plugins/blob/main/pstack/docs/guide/README.md>）を参照したクリーンルーム実装である。
