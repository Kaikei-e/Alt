---
name: adr-preflight
description: コード変更の前に DocDag の ADR グラフ健全性を検証し、変更先ディレクトリに効く現行契約（binding ADR）を提示して既存決定との無自覚な衝突を防ぐ。新機能実装・バグ修正・リファクタリング・改善の着手時（starting any feature, bug fix, refactor, or improvement）、編集フックが `ADR PREFLIGHT` または `ADR PREFLIGHT BLOCKED` で編集を保留したとき、あるいはユーザが「ADR 確認して」「preflight 回して」「check binding ADRs」と言ったときに、ユーザが「ADR」と言及しなくても起動する。設計前の横断的な vault 文脈収集には plan-context-loader、既存決定を置換する新規 ADR 執筆には alt-adr-writer を使用する。
allowed-tools:
  - "Bash(./scripts/altharness/alt-harness preflight *)"
  - Read
  - Grep
  - Glob
---

# ADR Preflight Gate

コード変更が既存の確定アーキテクチャ決定（binding ADR: `status: accepted` かつ未置換）と無自覚に衝突するのを編集前に防ぐゲートである。

## 1. 適用タイミングとスコープ

- **着手時**: 新機能・バグ修正・リファクタリング・改善のいずれであっても、コード変更を始める前に実行・確認する。
- **対象リポジトリとスコープ**: 本ゲートはこのリポジトリ本体とその git worktree のみに適用され、同一セッションで開いた別リポジトリのファイルは免除される。各 worktree は自身の `docs/ADR` に対して検証される。`PreToolUse` は git ルート直下の第 1 パス要素（ルート直下は `repo-root`）をスコープとし、`docs/` 配下とルート直下の `docdag.yaml`（両者は決定コーパスであり壊れたグラフをいつでも修復できるようにするため）、git-ignore 対象、リポジトリ外は免除する。
- **受領証のライフサイクル**: `SessionStart`（startup / resume / clear / compact）は当該セッションの受領証（receipt）を消去して `docdag validate` を実行し、`adr_preflight_enabled`（または `adr_preflight_disabled`）を 1 段落出力して常に exit `0` で終了する。同一スコープの 2 回目以降の編集は通過するが、コミット済みの `docs/ADR` ツリーまたは `docdag.yaml` が変化したとき、または `docs/ADR` 配下（あるいは `docdag.yaml`）の未コミットファイルが追加・削除・再変更されたとき、および `SessionStart` で受領証が失効するためコンテキスト圧縮後も再提示される。

## 2. フック出力の意味と対処手順

`PreToolUse` が exit `2` で編集を止めた場合、stderr の先頭で対応を分岐する:

1. **`ADR PREFLIGHT BLOCKED ... FIX: ...`**: 原因は `docdag validate` 失敗、`docdag` 不在、または `internal error`（ゲート自体の失敗＝fail-closed）のいずれか。解消されるまで免除対象以外の編集はすべてブロックされ続ける一方、グラフを修復できるよう `docs/` と `docdag.yaml` は編集可能なまま保たれるため、`docdag` の `fix:` / `FIX:` 行やエラー本文を読んで原因を修復してから再試行する。`internal error` に対する一時退避は `ALT_ADR_PREFLIGHT=disabled` だが、ユーザがその環境変数を設定してセッションを再起動する必要があるため、シェル書込で回避せずユーザへ報告する。
2. **`ADR PREFLIGHT (not an error)`**: グラフ健全。受領証を記録し、validate 要約・該当ディレクトリサービスの binding ADR 件数・最新最大 8 件（id + title）を示すため 1 回だけ編集を保留する。関係する ADR を `docdag context <id> --depth 2 --budget 1500` で確認してから同じ編集を再試行する（再試行は通過する）。

## 3. Binding ADR と矛盾する場合の原則

- 既存の binding ADR と矛盾する変更は**独断で実装せず**、直ちに作業を止めて衝突する ADR と理由をユーザへ報告する。
- 決定を更新する場合はユーザ合意のもと [.claude/skills/alt-adr-writer/SKILL.md](../alt-adr-writer/SKILL.md) で後継 ADR に `supersedes` を記述する。確定済み（accepted）ADR は追記専用であり、本文を書き換えてはならない（置換時の frontmatter `status: superseded` 更新のみ可）。

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
- **既知の制約**: シェルコマンド経由の編集（`sed -i`、リダイレクト等）は編集フックから見えないため、ファイル編集ツールを使うか事前に CLI を実行する。
- **より広い文脈収集**: 正規 contract・review・runbook を含む計画前の文脈収集は [.claude/skills/plan-context-loader/SKILL.md](../plan-context-loader/SKILL.md) を使う。

## 6. 配線（Wiring）

`.claude/settings.json` はバージョン管理外のため、Claude Code には次の 2 エントリを `hooks` に配線する:

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

- **Codex**: 追跡済みの `.codex/hooks.json` を参照するため、各ユーザが初回に `/hooks` で内容を確認・信頼承認してから有効化される。
- **フックなしエージェント**: `AGENTS.md` の手順どおり実装前に上記 CLI を手動実行する。
