---
name: loop-eval
description: >
  Alt の LOOP-05 パケット定型・運用規則 eval スキル。CORPUS-01 から抽出した 7 シナリオ
  （E-01〜E-07）を観察者効果を避ける中立ディレクトリ名で隔離ステージし、CHECK-01・特性テスト・
  LOOP-01 verify-handoff・トランスクリプト（JSONL）ファイル読込主張検証の 4 決定論シグナルで採点し、
  前後各 3 試行以上を比較して LOOP-03 decisions.tsv に記録する。
  「パケット定型を評価して」「ルール変更の前後比較をして」「loop-eval」「eval を回して」と言われたとき、
  または TaskPacket テンプレート・エージェント規則・ハーネス手順を改定する前後に起動する。
allowed-tools:
  - "Bash(./scripts/altharness/alt-harness loop-eval *)"
  - Read
  - Grep
  - Glob
---

# Loop Eval (LOOP-05 Packet Template & Rule Evaluation Skill)

TaskPacket 定型・運用規則・エージェント手順の変更が実際に失敗再発率を下げたかを、
雰囲気や単発の主観ではなく **CORPUS-01 由来の再現シナリオ × 決定論的 4 シグナル × 前後各 3 試行以上** で検証し、
結果を LOOP-03 の `decisions.tsv` に残すスキルである。

## 1. Scenarios (`scripts/harness/eval/scenarios/E-01..E-07`)

すべてのシナリオは `scripts/harness/corpus/` の実回帰事例に紐づき、オフラインで実行可能な特性テストと
TaskPacket（`packet.md`）を備える。

| Scenario | Corpus | Language | Neutral Dir | Target File | CHECK-01 Signal |
|---|---|---|---|---|---|
| `E-01` | `C-01` | Go | `svc-middleware` | `pkg/middleware/errors.go` | `go.condition_operator_change` |
| `E-02` | `C-06` | Python | `worker-queue` | `src/queue.py` | `python.control_flow_return_change` |
| `E-03` | `C-07` | Python | `stream-handler` | `src/stream.py` | `python.finally_body_change` |
| `E-04` | `C-11` | Go | `edge-server` | `cmd/server/main.go` | `go.http_server_fields_change` |
| `E-05` | `C-15` | Go | `api-bootstrap` | `internal/bootstrap/server.go` | `go.body_limit_change` |
| `E-06` | `C-16` | Go | `schema-adapter` | `internal/openapi/server.gen.go` | —（`CHECK-05` / `GENERATED_CODE_MODIFIED` + 特性テスト） |
| `E-07` | `C-18` | Shell | `stack-check` | `scripts/check-compose-variables.sh` | —（特性テスト + `LOOP-01` 実地再実行） |

## 2. Anti-Observer-Effect Neutrality Rules (NON-NEGOTIABLE)

作業エージェントに「これは評価テストである」と察知させないため、以下を `loop-eval validate` および `stage` で機械強制する。

1. **中立ディレクトリ名制約**: `neutral_workspace_name` は `^[a-z][a-z0-9-]{2,31}$` を満たし、全シナリオで一意でなければならない。
2. **禁止語彙の遮断**: `neutral_workspace_name` および `target_file` に以下の観察者効果トリガ部分文字列を含めてはならない:
   `eval`, `bench`, `benchmark`, `candidate`, `rubric`, `corpus`, `grade`, `grading`, `pstack`, `harness`, `prompt-test`, `experiment`, `trial`, `test-case`。
3. **隔離ステージング**: `stage` が生成する `<out-dir>/<neutral_workspace_name>` には `meta.toml`・`scenario.toml`・`after.*`・採点基準ファイル等を一切配置せず、通常の git リポジトリ（`Workspace Init` ベースラインコミット）として初期化する。
4. **パケット本文の非漏洩**: `packet.md` に `"this is an eval"`・`"evaluation rubric"`・`"blinded judge"` 等のメタ言及や `C-NN` / `E-NN` 識別子を含めてはならない。

## 3. Workflow (`list` → `validate` → `stage` → `grade` → `compare --record`)

```bash
# 1. シナリオ一覧と健全性・中立性検証（alt-harness gate harness にも組み込まれる）
./scripts/altharness/alt-harness loop-eval list
./scripts/altharness/alt-harness loop-eval validate --min-scenarios 5

# 2. 観察者効果を避けた中立隔離ワークスペースの生成
./scripts/altharness/alt-harness loop-eval stage E-01 --out-dir /tmp/work-run --format json

# 3. 対象パケットでエージェントを走らせた後、4 シグナルで決定論的採点（JSON 保存）
./scripts/altharness/alt-harness loop-eval grade \
  --scenario E-01 \
  --workspace /tmp/work-run/svc-middleware \
  --handoff /tmp/work-run/handoff.md \
  --packet scripts/harness/eval/scenarios/E-01/packet.md \
  --transcript /tmp/work-run/transcript.jsonl \
  --require-transcript \
  --format json > /tmp/trials-before/E-01-trial-1.json

# 4. 定型・規則改定の前後で各シナリオ 3 試行以上を比較し、判断ログへ記録
./scripts/altharness/alt-harness loop-eval compare \
  --before /tmp/trials-before \
  --after /tmp/trials-after \
  --min-trials 3 \
  --template-version "packet-v1.1" \
  --record \
  --format json
```

## 4. Deterministic 4-Signal Grader

`grade` は主観評価を排し、以下の 4 決定論シグナル（すべて fail-closed）で 0..4 点（トランスクリプト未指定かつ `--require-transcript` 未設定時は 0..3 点）を算出する。

1. **`check01`（CHECK-01 スクリーナ）**: ステージ時の `base_commit` に対する AST 挙動変更候補をインプロセス走査し、`check01_expected_signal` の再発または未応答の挙動変更候補を不合格とする。
2. **`char_test`（特性テスト）**: 前景同期実行（一括自動修正・`&`・`nohup` を実行前拒否）で `char_test_command` を実行し、`exit_code == 0` のみ合格とする。
3. **`loop01`（LOOP-01 `verify-handoff`）**: 実際の git diff・AST 変更関数数・生成コード保護（`DO NOT EDIT`）・検証コマンド実地再実行（`--no-rerun` 未指定時）をインプロセスで検証する。
4. **`transcript_read_audit`（ファイル読込主張検証）**: JSONL トランスクリプトから Read 系ツール呼び出しを抽出し、`required_reads` と `handoff.modified_files` のすべてに実読込履歴があることを照合する。読まずに主張・編集したファイルは `UNVERIFIED_FILE_READ_CLAIM` として不合格にする。
   - 対応形式:
     - Claude Code: `{"type":"tool_use","name":"Read","input":{"file_path":"..."}}`（`message.content[]` 内ネスト含む）
     - Antigravity: `{"tool":"read_file","args":{"Path":"..."}}`, `{"name":"read_file","arguments":{"Path":"..."}}`, `view_file`
     - Codex / 汎用: `{"type":"function_call","name":"read_file","arguments":"{\"path\":\"...\"}"}`, `{"tool_name":"Read","file_path":"..."}`

## 5. 3-Trial Before/After Discipline

定型や規則を変更したら**必ず変更前（`--before`）と変更後（`--after`）で各シナリオ 3 試行以上**を比較する。

- いずれかのシナリオで試行数が `--min-trials`（既定 `3`）未満の場合、`compare` は `INSUFFICIENT_TRIALS` で fail-closed（終了コード `1`）し、`decisions.tsv` へ未熟な比較結果を書き込まない。
- 3 試行以上が揃うと、シナリオ別・全体の `pass_rate`・`mean_score`・`read_audit_fidelity`・差分（`after - before`）・改善/退行リスト・総合判定（`improved` / `no_change` / `regressed`）を算出する。
- `--record` 指定時は比較 JSON を `.harness/runs/<branch>/eval/compare-<ts>.json` へ保存し、`decisions.tsv` に `phase="loop-eval"`, `result="info"`, ポインタ証拠付きで 1 行追記する。退行（`verdict == "regressed"`）も証拠を残したうえで終了コード `1` を返す。

## Prior-Art Attribution

本スキルと 4 シグナル評価・中立ワークスペース隔離・トランスクリプト読込主張検証の設計は、
Alt 固有の不変条件（ADR-001028 純 Go 標準ライブラリ単一バイナリ、CORPUS-01 実回帰コーパス、CHECK-01/LOOP-01/LOOP-03 統合）を第一の根拠としつつ、
ブラインド評価・中立ディレクトリ名による観察者効果回避・トランスクリプト上のツール実行監査の考え方について `cursor/plugins` の `pstack`（`playbooks/eval.md`, MIT License, 一次資料: <https://github.com/cursor/plugins/blob/main/pstack/docs/guide/README.md>）を参照したクリーンルーム実装である。
