---
name: correct
description: >
  Alt の LOOP-07 `/correct` 区切り是正ループスキル。各サービスのマイルストーン区切り、または同じ失敗クラスが 2 回以上再発したときに起動し、
  `decisions.tsv` の `reverted` / `mismatch` 行から再発クラスを抽出・`CORPUS-01` へ登録し、`ARCH → CHECK → VERIFY → LEDGER` の
  最上位段から対処を決めて実失敗での検知を実証したうえで、Markdown/JSON/判断ログへ記録し `rule-ledger.md` を更新・不要な文章ルールを削除する。
  「/correct」「区切りで是正して」「同じ失敗が 2 回起きた」「loop-correct」と言われたときに起動する。
allowed-tools:
  - "Bash(./scripts/altharness/alt-harness loop-correct *)"
  - "Bash(./scripts/altharness/alt-harness corpus *)"
  - "Bash(./scripts/altharness/alt-harness selfcheck *)"
  - "Bash(git diff *)"
  - "Bash(git status *)"
  - Read
  - Grep
  - Glob
  - Edit
  - Write
---

# Correct (LOOP-07 Milestone Correction Cadence)

各サービスのマイルストーン区切り、または同種の失敗が **2 回以上** 再発した時点で、その失敗を「単発のミス」ではなく**クラス**として昇格させ、より強い強制段（`ARCH → CHECK → VERIFY → LEDGER`）へ落とし込むための運用ループである。

> **Two-Strike Class Rule（2 回でクラス）**: 1 回の失敗は単発として観察にとどめる。2 回以上起きた失敗は必ずクラス化し、`CORPUS-01` へ実例を登録し、上位段から順に構造化・機械化する。
>
> **Proven-to-Fail Rule（実失敗での実証必須）**: 追加した構造制約・静的検査・動的検証は、登録した `CORPUS-01` の `after.<ext>`（過去の実際の失敗）で確実に **FAIL** し、`before.<ext>`（安全な状態）で **PASS** することを実証するまで完了とみなさない。

## 1. When to Run

以下のいずれかに該当したときに本スキルを起動する:

1. **サービス単位のマイルストーン区切り**: 1 サービス（例: `auth-hub`, `alt-backend`, `news-creator` 等）のリント適用・機能群・大規模リファクタリングが完了した直後。
2. **再発トリガ（2 回以上）**: `.harness/runs/<branch>/decisions.tsv`（または全ブランチの `.harness/runs/*/decisions.tsv`）で、同一失敗クラスの `reverted` / `mismatch` が 2 回以上に達したとき。
3. **監査指摘の横展開**: LOOP-04（`/interrogate`）やポストモーテムで同型ミスが複数箇所から見つかったとき。

## 2. The Rung Ladder (Strict Precedence)

対処手段は必ず上位から検討し、**実現可能な最も上位の段**を選ぶ:

| 優先順位 | 段 (`chosen_rung`) | 何をするか | Alt での具体例 |
|---:|---|---|---|
| 1（最優先） | `ARCH` | **構造的に書けなくする**（型・正規コンストラクタ・境界ラッパ・`ast-grep` 禁止ルール） | `ast-grep:go-no-raw-http-server`, `internal/httpserver.New`, `fsperm.File`, `ast-grep:python-no-manual-aclose-in-finally` |
| 2 | `CHECK` | **静的検査で自動拒否する**（AST スクリーナ・抑制ラチェット・フック・依存方向検査） | `scripts/altharness/alt-harness screener`, `alt-harness suppressions`, `alt-harness ratchet`, `alt-harness generated`, `.agents/skills/clean-architecture/scripts/check_layers.sh` |
| 3 | `VERIFY` | **動的検証・実機プローブで落とす**（特性テスト・乱数差分試験・ゴールデン・ライブ検証） | `scripts/altharness/alt-harness verify-golden`, `alt-harness verify-diff`, `scripts/verify/verify-live.sh`, `alt-harness flaky-hunt` |
| 4（最終手段） | `LEDGER` | **規則台帳へ明示登録する**（上位 3 段がすべて適用不能な場合のみ） | `.agents/rules/rule-ledger.md#R-XX`（`max_judgment_rules` の上限内で管理） |

### Higher-Rung Rejection Invariant (NON-NEGOTIABLE)

選んだ段より**厳密に上位のすべての段**について、「なぜその段では防げなかったのか」を具体的技術根拠（12 文字以上、`TBD` / `N/A` / `hard` / `too hard` / `cannot` 等の形骸表現は機械拒否）として `higher_rungs_rejected` に記録する:

- `ARCH` を選択 → 上位段なし（`higher_rungs_rejected: {}`）。
- `CHECK` を選択 → **`ARCH`** を使わなかった理由が必須。
- `VERIFY` を選択 → **`ARCH` と `CHECK`** を使わなかった理由が必須。
- `LEDGER` を選択 → **`ARCH`・`CHECK`・`VERIFY`** を使わなかった理由がすべて必須。

## 3. Workflow (6 Steps)

```text
LOOP-07 /correct Cadence:
- [ ] Step 1: Scan decisions.tsv for recurring failure classes (count >= 2)
- [ ] Step 2: Promote unmapped recurring classes to CORPUS-01 (scaffold-corpus)
- [ ] Step 3: Implement the highest feasible rung (ARCH -> CHECK -> VERIFY -> LEDGER)
- [ ] Step 4: Prove the new enforcement fails on CORPUS-01 after.<ext> and passes on before.<ext>
- [ ] Step 5: Validate and record milestone disposition (Markdown + JSON + decisions.tsv)
- [ ] Step 6: Update CHECK-07 rule-ledger.md and delete obsolete prose rules
```

### Step 1: Scan Recurring Failure Classes

まず `decisions.tsv` の `reverted` / `mismatch` 行を集計し、2 回以上発生したクラスと既存コーパス登録状況を抽出する:

```bash
# 現在のブランチを走査（既定 threshold=2）
./scripts/altharness/alt-harness loop-correct scan --format json

# 全ブランチを横断走査
./scripts/altharness/alt-harness loop-correct scan --runs-dir .harness/runs --threshold 2 --format json

# 特定の decisions.tsv を指定して走査
./scripts/altharness/alt-harness loop-correct scan --decisions-file .harness/runs/<branch>/decisions.tsv --format text
```

クラスキーは以下の決定論的優先順位で抽出される:
1. `decision` / `why` 内の明示トークン `class=<id>`（例: `class=errorlint-and-drop`, `class=C-01`）
2. `decision` / `why` / `evidence` 内のコーパス ID `C-NN`
3. `decision` / `why` 内の角括弧エラーコード `[CODE_NAME]`（例: `[VERIFICATION_EXIT_CODE_MISMATCH]`）
4. `phase` と `why` 第 1 節からの正規化フォールバック署名

`single_occurrences`（1 回のみ）は情報表示にとどめ、原則として `recurring_classes`（2 回以上）のみを対処対象とする。

### Step 2: Promote Unmapped Classes to `CORPUS-01`

`corpus_mapped: false` の再発クラスについて、次番号 `C-NN` の雛形を確認・生成する:

```bash
# まずドライランで生成内容と採番（例: C-23）を確認
./scripts/altharness/alt-harness loop-correct scaffold-corpus \
  --class-id "<class_id>" \
  --title "<short title of the mistake>" \
  --language "<go|python|rust|typescript|shell|binary>" \
  --category "<valid corpus category>"

# 問題なければ --write で新規 C-NN ディレクトリを作成（既存ディレクトリは決して上書きしない）
./scripts/altharness/alt-harness loop-correct scaffold-corpus \
  --class-id "<class_id>" \
  --title "<short title of the mistake>" \
  --language "<lang>" \
  --category "<category>" \
  --write
```

作成された `scripts/harness/corpus/C-NN/` の `before.<ext>`（安全側）・`after.<ext>`（過去の実際の失敗）および `meta.toml` の `[origin]` / `[detection]` を実証拠で埋める。

### Step 3: Implement the Highest Feasible Rung

ラダー順（`ARCH → CHECK → VERIFY → LEDGER`）に最小で最も強力な強制手段を実装する:
- `ARCH`: `scripts/lint/ast-grep/rules/<id>.yml` または安全な正規コンストラクタ／型境界。
- `CHECK`: `scripts/altharness/internal/screener` 等の静的検査・フック。
- `VERIFY`: `verify-golden` / `verify-diff` / `verify-live` / 特性テスト。
- `LEDGER`: `.agents/rules/rule-ledger.md` への登録（上位 3 段すべてに具体的な却下理由がある場合のみ）。

本番コードやハーネスを変更する場合は ADR-001032 に従い ADR ゲートを通過させる。

### Step 4: Prove It Fails on the Real Past Mistake

作成した強制手段が、実際に過去の失敗で発火することを必ず検証する:

```bash
# CORPUS-01 全体（before=PASS, after=FAIL）の回帰検証
./scripts/altharness/alt-harness corpus validate
./scripts/altharness/alt-harness corpus evaluate
```

`after.<ext>` を検知できないチェックや、`before.<ext>` で誤検知するチェックは未完成であり、Step 5 へ進んではならない。

### Step 5: Validate and Record the Disposition

マイルストーン対処 JSON（例: `.harness/runs/<branch>/correct-draft.json`）を作成する:

```json
{
  "milestone_id": "service-auth-hub-2026-11-28",
  "service": "auth-hub",
  "classes": [
    {
      "class_id": "C-11",
      "title": "Unsolicited IdleTimeout added to http.Server",
      "occurrences": 2,
      "evidence": [
        ".harness/runs/main/decisions.tsv",
        "docs/review/lint-rollout-harness-learnings-2026-11-21.md#2.2"
      ],
      "corpus_id": "C-11",
      "chosen_rung": "ARCH",
      "higher_rungs_rejected": {},
      "enforcement_ref": "ast-grep:go-no-raw-http-server",
      "notes": "Canonical httpserver.New entrypoint + ast-grep rule catches C-11."
    }
  ]
}
```

fail-closed 検証と記録を実行する:

```bash
# 1. 厳格検証（不正な段・2回未満・存在しないコーパス/強制手段・上位段却下理由欠落をすべて拒否）
./scripts/altharness/alt-harness loop-correct validate <disposition.json> --format json

# 2. Markdown 表 + JSON 保存 + decisions.tsv 追記（1 クラス 1 行、phase=correct, result=info）
./scripts/altharness/alt-harness loop-correct record <disposition.json> --format json
```

`record` は検証を通過した場合のみ以下を出力する:
- `.harness/runs/<branch>/correct/<milestone_id>.md`（正準 7 列 Markdown 表: `| クラス | 発生回数 | 証拠 | 選んだ段 | 上の段を使わなかった理由 | コーパス | 強制手段 |`）
- `.harness/runs/<branch>/correct/<milestone_id>.json`
- `.harness/runs/<branch>/decisions.tsv` への追記行（`evidence` は生成した Markdown 表へのリポジトリ相対パス）

### Step 6: Update `CHECK-07` `rule-ledger.md` & Delete Obsolete Prose Rules

1. `.agents/rules/rule-ledger.md` に新設した機械的強制手段（`ast-grep:<id>` / `scripts/altharness/alt-harness <subcommand>` / 実在スクリプトパス）を登録する。
2. 既存の `判断` 行を機械化できた場合は、`rule-ledger.md` の `max_judgment_rules` と `scripts/lint/ratchet/suppressions.toml` の `[governance].max_judgment_rules` を**同一コミットで引き下げる**。
3. **不要な文章ルールを削除する**: `ARCH` 層で構造的に不可能になった（あるいは `CHECK` で常時自動拒否される）古い注意書きや文章ベースの禁止事項は、`CLAUDE.md` / `AGENTS.md` / `.agents/rules/` から積極的に削除する（文章の蓄積ではなく、構造と検査へ置換するのが `/correct` の目的である）。
4. 最後に自己診断を通して全参照の健全性を確認する:

```bash
./scripts/altharness/alt-harness selfcheck
```

## Prior-Art Attribution

本スキルと LOOP-07 是正エンジンは、Alt 自身の lint 展開失敗記録（`docs/review/lint-rollout-harness-learnings-2026-11-21.md`）・失敗コーパス（`CORPUS-01`）・規則台帳ラチェット（`CHECK-07`）・純 Go 標準ライブラリ単一バイナリ方針（ADR-001028 / ADR-001038）を第一の根拠として設計しつつ、2 回再発でのクラス化・上位段優先の強制ラダー（architecture → static check → verification → prose）・実失敗での検知実証・不要な文章ルール削除の運用様式について、`cursor/plugins` の `pstack`（`/correct`, MIT License、一次資料: <https://github.com/cursor/plugins/blob/main/pstack/docs/guide/README.md>）を参照したクリーンルーム実装である。
