---
name: adr-preflight
description: 本番コード編集の前に DocDag の ADR グラフ健全性を検証し、変更先ディレクトリに効く現行契約（binding ADR）を確認したうえで有効な `docs/ADR/NNNNNN.md`（`status: proposed`）の先行起票を強制するゲート。新機能実装・バグ修正・リファクタリング・改善の着手時（starting any feature, bug fix, refactor, or improvement）、編集フックが `ADR PREFLIGHT BLOCKED` で本番編集を止めたとき、あるいはユーザが「ADR 確認して」「preflight 回して」「check binding ADRs」と言ったときに、ユーザが「ADR」と言及しなくても起動する。設計前の横断的な vault 文脈収集には plan-context-loader、ADR の実装前ドラフト作成とテスト通過後の確定昇格には alt-adr-writer を使用する。
allowed-tools:
  - "Bash(./scripts/altharness/alt-harness preflight *)"
  - Read
  - Grep
  - Glob
---

# ADR Preflight Gate（ADR 先行ゲート）

本番コード変更が既存の確定アーキテクチャ決定（binding ADR: `status: accepted` かつ未置換）と無自覚に衝突するのを防ぎ、**本番コード編集前に `docdag` で検証済みの `docs/ADR/NNNNNN.md` を起票させる**機械ゲートである（[[001032]]）。

## 1. 適用タイミング・免除対象・自動解除

- **着手時（本番コード編集前）**: 新機能・バグ修正・リファクタリング・改善のいずれであっても、本番コードを変える前に binding ADR を確認し `docs/ADR/NNNNNN.md`（`status: proposed`）を起票する。
- **対象リポジトリと免除対象**: 本ゲートはこのリポジトリ本体とその git worktree のみに適用され、リポジトリ外と git-ignore 対象は免除される。リポジトリ内の免除対象は **`docs/**`・`docdag.yaml`**（決定コーパス自体）と **テストファイル**（`*_test.go`、`test_*.py` / `*_test.py` / `conftest.py`、`*.test.*` / `*.spec.*` / `*_test.ts(x)`、`*_test.rs`、`*_test.sh` / `test-*.sh`、および `tests/` / `test/` / `__tests__/` / `e2e/` / `testdata/` / `fixtures/` 配下）のみである。
  - テストファイルのみの編集は ADR 起票を免除され TDD の RED テスト先行作成を妨げないが、グラフ破損（`docdag validate` 失敗）時は fail-closed でブロックされる。
  - 1 回のパッチにテストと本番ファイルが混在する場合は本番ファイル側で ADR が必須となる。
- **無条件リトライ不可・`--skip` なし**: 有効な ADR が存在しない限り `PreToolUse` は受領証を一切書き込まず exit `2` でブロックし続ける。同一編集をそのままリトライしても通過しない。`--skip` 等の迂回 CLI も存在しない。
- **自動解除と再ロック**: 作業ツリーに未コミットの有効な `docs/ADR/NNNNNN.md`（削除以外の非空通常ファイル）、または非デフォルトブランチ（`HEAD` が `main`/`master` 以外）の `<base>...HEAD` 差分に `AMCR` の `docs/ADR/NNNNNN.md` が存在し、かつ `docdag validate` が通過すれば、受領証を記録して即通過する（二重保留なし）。`main`/`master` の既コミット済み ADR は対象外である。また、ADR が削除・破損された場合はフィンガープリント変化により受領証が失効し即座に再ロックされる。
- **SessionStart**: `SessionStart`（startup / resume / clear / compact）は当該セッションの受領証を消去し、`docdag validate` を実行して `adr_preflight_enabled`（または `adr_preflight_disabled`）と ADR 先行契約を 1 段落出力して exit `0` で終了する。

## 2. フック出力の意味と対処手順

`PreToolUse` が exit `2` で編集を止めた場合、stderr の見出しで対応を分岐する:

1. **`ADR PREFLIGHT BLOCKED (ADR required before code edits)`**: グラフは健全だが、現行作業ツリー／非デフォルトブランチ差分に有効な `docs/ADR/NNNNNN.md` がまだ存在しない。受領証は記録されないため、**そのまま再試行せず** stderr に示された 5 手順を実行する:
   1. 提示された binding ADR を `docdag context <id> --depth 2 --budget 1500` で確認する（既存決定と矛盾する場合は実装せずユーザへ確認する）。
   2. `docdag new "<title>" --dry-run --format json` で次の 6 桁 ID とパスを取得する。
   3. `docs/ADR/template.md` から `docs/ADR/NNNNNN.md` を `status: proposed`（`## Status\n\nProposed`）で作成する（詳細は [alt-adr-writer/SKILL.md](../alt-adr-writer/SKILL.md)）。
   4. `docdag validate --touching docs/ADR/NNNNNN.md` を通す。
   5. 本番編集を再試行する（有効な ADR が検知され、即通過する）。
2. **`ADR PREFLIGHT BLOCKED ... FIX: ...`（グラフ異常・`docdag` 不在・内部エラー）**: `docdag validate` 失敗、`docdag` 不在、または `internal error`（fail-closed）。解消されるまで `docs/**`・`docdag.yaml` 以外の編集（テスト含む）はすべてブロックされる。`docs/` と `docdag.yaml` は修復用に編集可能なまま保たれるため、`docdag` の `fix:` / `FIX:` 行を読んで修復してから再試行する。`internal error` や壊れたツールチェーンでの一時退避が必要な場合はシェル書込で迂回せずユーザへ報告する。

## 3. Binding ADR と矛盾する場合の原則

- 既存の binding ADR と矛盾する変更は**独断で実装せず**、直ちに作業を止めて衝突する ADR と理由をユーザへ報告する。
- 決定を更新する場合はユーザ合意のもと [alt-adr-writer/SKILL.md](../alt-adr-writer/SKILL.md) の 2 段階手順に従う。`docdag.yaml` の `status_drift` 規則のため、`status: proposed` のドラフト段階では本文に `[[000NNN]]` を明記し frontmatter の `supersedes:` は省略する。テスト green 後の `status: accepted` 昇格時に `supersedes: ["000NNN"]` を追加し、同一コミットで旧 ADR の frontmatter のみ `status: superseded` に更新する（確定済み ADR の本文 Decision は改変しない）。

## 4. CLI 実行と終了コード（フックなし環境・人間用）

```bash
# 変更予定パスまたは変更済みファイルからブリーフを stdout に出力（exit 0、docdag validate 失敗または docdag 不在時は exit 1）
./scripts/altharness/alt-harness preflight <path>...
./scripts/altharness/alt-harness preflight --changed
# 指定サービスを affected_services に持つ全 binding ADR を新しい順に出力（id<TAB>title<TAB>path）
./scripts/altharness/alt-harness preflight --service <name>
```

## 5. 緊急停止スイッチ・既知の制約・関連スキル

- **緊急停止スイッチ（Kill switch）**: 環境変数 `ALT_ADR_PREFLIGHT=disabled`（`SessionStart` が `adr_preflight_disabled` を出力）。壊れたツールチェーン専用の退避策であり、利便性や確認省略のために使ってはならない。
- **既知の制約**: シェルコマンド経由の直接ファイル書込（`sed -i`、リダイレクト等）は編集フックから見えないため、ファイル編集ツールを使うか事前に CLI を実行する。
- **関連スキル**:
  - ADR の実装前ドラフト作成・テスト通過後の確定昇格: [alt-adr-writer/SKILL.md](../alt-adr-writer/SKILL.md)
  - 正規 contract・review・runbook を含む計画前の文脈収集: [plan-context-loader/SKILL.md](../plan-context-loader/SKILL.md)

## 6. 3 環境の配線（Wiring）

3 つのエージェント環境すべてで git 追跡済みのフック設定により同一の `alt-harness preflight` ゲートを適用する:

- **Claude Code（`.claude/settings.json` — git 追跡済み）**: `.gitignore` の `!.claude/settings.json` により新規クローンおよび Claude Code worktree でも自動継承される（個人設定・ローカル許可は git 管理外の `.claude/settings.local.json` に分離）。`PostToolUse`（`Edit|Write`）では `docs/ADR/*.md` に `docdag validate --touching`、`docdag.yaml` に `docdag lint --strict --config` を即時実行する（違反時 exit `2`）。`SessionStart` / `PreToolUse` の `preflight` 配線抜粋:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR/scripts/altharness/alt-harness\" preflight --hook",
            "timeout": 30
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Edit|Write|NotebookEdit",
        "hooks": [
          {
            "type": "command",
            "onFailure": "block",
            "command": "\"$CLAUDE_PROJECT_DIR/scripts/altharness/alt-harness\" preflight --hook",
            "timeout": 30
          }
        ]
      }
    ]
  }
}
```

- **Antigravity CLI / `agy-cli`（`.agents/hooks.json` — git 追跡済み）**:
  - `SessionStart`: `alt-harness agy-hook session-start`
  - `PreToolUse`: `alt-harness agy-hook pre-edit`（`preflight` が 0 以外を返した場合は常に fail-closed で `decision: "deny"`）
  - `Stop`: `alt-harness agy-hook stop`（Clean Architecture 層検査に加え、`docs/ADR/`・`docdag.yaml` に未コミット変更があれば `docdag validate` も検証）
- **Codex（`.codex/hooks.json` — git 追跡済み）**: `SessionStart` と `PreToolUse` (`apply_patch|Edit|Write`) で `alt-harness preflight --hook` を実行（各ユーザが初回 `/hooks` で確認・承認して有効化）。
- **フックなし環境**: `AGENTS.md` の手順どおり実装前に `./scripts/altharness/alt-harness preflight <paths>` を手動実行し、ドラフト ADR を作成してから編集する。
