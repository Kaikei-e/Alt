---
name: maintain-verification-skill
description: >
  Alt のライブ検証スキル（.claude/skills/verify-alt/）と Feature Map（features/*.md）を実コードと突き合わせて
  最新状態に保ち、必須ライブ実行で破綻がないことを確認する保守スキル。
  機能追加・ルート変更・契約変更の後や、「verify-alt を更新して」「feature map を最新化して」
  「maintain-verification-skill」と言われたときに起動する。プロダクトコードは決して無断で書き換えない。
allowed-tools:
  - "Bash(./scripts/verify/verify-live.sh *)"
  - "Bash(./scripts/altharness/alt-harness verify-live *)"
  - "Bash(git diff *)"
  - "Bash(git status *)"
  - Read
  - Grep
  - Glob
  - Edit
  - Write
---

# Maintain Verification Skill (`verify-alt`)

`.claude/skills/verify-alt/SKILL.md` と `.claude/skills/verify-alt/features/*.md` が、
リポジトリの実コード・現行 ADR・ライブ検証ハーネスから乖離していないかを 5 段階で点検・修復するループである。

> **Non-negotiable rule**: このスキルは検証スキルと Feature Map のみを保守する。**プロダクトコード（サービス実装・設定・マイグレーション等）を無断で修正してはならない。** プロダクト側の不具合・ADR 衝突・実行環境欠損を見つけた場合は修正せず `blocked` として報告する。

## Workflow (5 Phases)

```text
Maintain Verification Skill:
- [ ] Phase 1: Index Hygiene (`verify-live doctor`)
- [ ] Phase 2: Read-only Source Wave (inspect real code & recent ADRs)
- [ ] Phase 3: Reconcile (update verify-alt SKILL.md & features/*.md only)
- [ ] Phase 4: Mandatory Live Pass (`verify-live run --probe all` + `check-c07-corpus`)
- [ ] Phase 5: Emit Outcome (`clean | changed | blocked`)
```

### Phase 1: Index Hygiene

まず `./scripts/verify/verify-live.sh doctor --format json` を実行する。
以下を機械的に確認する:
- 隔離不変条件（`COMPOSE_PROJECT_NAME != alt`）が満たされていること。
- `.claude/skills/verify-alt/SKILL.md` と `.agents/skills/verify-alt` シンボリックリンクが有効であること。
- `features/*.md`（`README.md` 除く）に孤児ファイルが無く、`features/README.md` にリンク切れが無いこと。
- 各 `features/*.md` が必須 4 セクション（`## Sub-features`, `## How to get to it`, `## Driving it with the harness`, `## Gotchas`）を備えていること。

### Phase 2: Read-only Source Wave

古いドキュメントを信じず、実コードを読み取り専用で確認する:
1. `news-creator/app/news_creator/handler/{sse_stream.py,summarize_handler.py}` — SSE ヘッダ・`aclosing` 経路・エラー文言。
2. `pre-processor/app/bootstrap/server.go` — `apiBodyLimitBytes` と `/api/v1/*` ルート群。
3. `auth-hub/cmd/auth-hub/main.go` と `auth-hub/internal/adapter/handler/{validate,session,health}.go` — ルート・`X-Alt-*` ヘッダ・平文制限。
4. `docker/inference-proxy/main.go` — 許可ルート・`Rewrite` ヘッダ転送・`Authorization` 除去・`DefaultMaxRequestBodyBytes`。
5. `alt-frontend-sv/src/routes/`・`src/lib/config/navigation.ts`・`src/lib/components/desktop/layout/Sidebar.svelte` — 現行ルート・リダイレクト・廃止ルート（ADR-001022 / ADR-001026 以降の最新 ADR も `git log -n 10 -- docs/ADR/` で確認）。

### Phase 3: Reconcile

Phase 1・2 で検出したドキュメント乖離のみを修正する:
- **編集してよい範囲**: `.claude/skills/verify-alt/SKILL.md` および `.claude/skills/verify-alt/features/*.md` のみ。
- **編集してはいけない範囲**: いかなるサービス実装コード・本番設定・`main.go`・`rule-ledger.md` 等も編集しない。
- 新規サーフェスが追加されていれば 4 セクション規約で `features/<name>.md` を追加し、`features/README.md` の索引表に追記する。
- 廃止されたルートやエンドポイントは現行一覧から除去し、必要なら `Gotchas` に ADR 番号付きで廃止注記を残す。

### Phase 4: Mandatory Live Pass

ドキュメント更新の有無にかかわらず、必ずライブ実行を完走させて実証拠を残す:

```bash
./scripts/verify/verify-live.sh doctor
./scripts/verify/verify-live.sh run --task maintain-pass --probe all
./scripts/verify/verify-live.sh check-c07-corpus \
  --before scripts/harness/corpus/C-07/before.py \
  --after scripts/harness/corpus/C-07/after.py
```

確認項目:
- すべて終了コード `0`。
- `.harness/evidence/maintain-pass/summary.json` と各プローブの `*.json` / `*.log` が生成され、全プローブが `ok: true` であること。
- `git status` に未追跡の一時ファイルやバイナリが残留していないこと（`.harness/` は `.gitignore` 済み）。

### Phase 5: Outcome Classification

最終報告は以下の 3 値のいずれかを先頭に明記する:

- **`clean`**: 乖離なし、`verify-live doctor`・全ライブプローブ・C-07 コーパス検証がすべて成功し実証拠あり。
- **`changed`**: `.claude/skills/verify-alt/` 配下の乖離を修復し、修復後の必須ライブ実行と証拠生成がすべて成功。
- **`blocked`**: プロダクトコードのバグ（ライブプローブ失敗）、隔離違反（`COMPOSE_PROJECT_NAME=alt`）、必須ツール欠損、または binding ADR との矛盾が見つかり、検証スキル側だけでは解消できない状態。プロダクトコードは編集せず、再現コマンドと証拠パスを添えてエスカレーションする。

## Prior-Art Attribution

本保守ループ（Index hygiene → Read-only Source Wave → Reconcile → Mandatory Live Pass → `clean | changed | blocked`）は、Alt の本番隔離・ADR 厳守・純 Go 検証ハーネス（ADR-001028）に合わせて設計しつつ、`cursor/plugins` `pstack` の `maintain-verification-skill`（MIT License、一次資料: <https://github.com/cursor/plugins/blob/main/pstack/docs/guide/README.md>）を参照したクリーンルーム実装である。
