---
name: verify-alt
description: >
  Alt のライブ検証スキル。本番 Compose（-p alt）を一切触らずに隔離された 127.0.0.1:0 ループバック実サーバーまたは
  隔離 Compose スタックを起動し、実 HTTP/SSE/認証/プロキシ経路を駆動して `.harness/evidence/<task>/` に再現可能な証拠を残す。
  「動かして検証して」「ライブ検証して」「verify-alt」「prove it works」「実 HTTP で確認して」と言われたとき、
  またはストリーミング・BodyLimit・認証・プロキシ・UI ルートを変更した後に起動する。
allowed-tools:
  - "Bash(./scripts/verify/verify-live.sh *)"
  - "Bash(./scripts/altharness/alt-harness verify-live *)"
  - Read
  - Grep
  - Glob
---

# Verify Alt (Live Isolated Verification Skill)

変更した振る舞いが単体テストのモックだけでなく**実ソケット・実 HTTP・実ハンドラ経路**で動作することを、
本番環境（`-p alt`）から完全に隔離したうえで証明し、永続証拠を `.harness/evidence/<task>/` に残すスキルである。

> **Not a Draft Rule**: 検証手順は最低 1 回最後まで実行し、`.harness/evidence/<task>/` に実証拠が出力された時点で初めて有効とみなす。未実行の手順は下書きであり、完了報告の根拠に使ってはならない。

## 1. Surface

本スキルが対象とする Alt の検証面と各機能のマップは [features/README.md](features/README.md) を正本とする。

| Probe / Surface | Target Service | What It Proves Live | Feature Doc |
|---|---|---|---|
| `sse` | `news-creator/app` | `news_creator.handler.sse_stream` の実 FastAPI/Uvicorn HTTP SSE ストリームが `aclose()` 例外時でもハングせず `event: error` と終端 EOF を返すこと（C-07） | [features/summarize-sse.md](features/summarize-sse.md) |
| `body-limit` | `pre-processor/app` | Echo v5 実 HTTP サーバー上で `/api/v1/summarize` が `1,000,000` bytes を受理（`!= 413`）し、`1,000,001` bytes を `413` で拒否すること（C-15） | [features/body-limit.md](features/body-limit.md) |
| `auth-hub` | `auth-hub` | Echo v5 実 HTTP サーバー＋ローカル Kratos スタブ上で `/health` 200、未認証 `/validate` 401、認証済み `/validate` 200＋`X-Alt-User-Id`/`X-Alt-Tenant-Id`/`X-Alt-Backend-Token`、`/session` 200 を返すこと | [features/auth-hub-session.md](features/auth-hub-session.md) |
| `inference-proxy` | `docker/inference-proxy` | 実 ReverseProxy が未認証 401・管理ルート 403・正当 Bearer 転送時の 6 ヘッダ/URL 不変条件（`Host`, `X-Forwarded-For` 追記, `X-Forwarded-Host`/`Proto` 非合成, path+query, カスタムヘッダ保持, `Authorization` 除去）を満たすこと（C-14） | [features/inference-proxy-headers.md](features/inference-proxy-headers.md) |
| `frontend-surfaces` | `alt-frontend-sv` | ADR-001022 / ADR-001026 以降の現行ルート構成（`/` → `/today`、モバイル 5 タブ、メニュー、デスクトップ Sidebar） | [features/frontend-surfaces.md](features/frontend-surfaces.md) |

## 2. Isolation Invariant (NON-NEGOTIABLE)

Alt の本番スタックは同一ホスト上で Docker Compose プロジェクト **`-p alt`** として稼働している。
検証ハーネスは以下の隔離不変条件を**コードと `doctor` の両方で強制**する。

1. **本番プロジェクト `-p alt` の全面禁止**:
   - `COMPOSE_PROJECT_NAME=alt` または `--project alt` が指定された場合、`verify-live doctor` および `verify-live run` は即座に fail-closed（終了コード 1）で拒否する。
   - 本番ポート（`80`, `4173`, `4433`, `5432`, `5434`, `7700`, `8888`, `9000`, `9200`, `9250`, `11434`, `11436` 等）、本番 DB、本番 Docker ボリュームには一切触れない。
2. **Mode A — Isolated Loopback Live Probes（既定・オフライン対応）**:
   - すべての実サーバーとローカルスタブは **`127.0.0.1:0`**（エフェメラルポート、ループバック限定、`0.0.0.0` 禁止）にバインドする。
   - 一時ディレクトリは `os.MkdirTemp` で隔離し、終了時にすべて確実に削除する。Docker デーモンや外部ネットワークは不要。
   - リポジトリ配下にビルド成果物や一時ファイルを一切残さない（`alt-harness hygiene` 準拠）。
3. **Mode B — Isolated Compose Verification Stack（明示オプトイン・`guard-ops` 準拠）**:
   - フルスタック・ブラウザ E2E 等で Compose が必要な場合のみ使用する。
   - 必ず固有の非本番プロジェクト名 **`-p alt-verify-<run_id>`** を明示し、本番と衝突しないホストポート帯（`19xxx`）と使い捨てボリュームのみを使う。
   - 実行前後に `guard-ops` 規則（O1–O4）を遵守する:
     - `docker system prune` / `docker volume prune` 等の無差別掃除（O1）は絶対に使わない。
     - `-p alt` に対する `up` / `down` / `rm` / `restart` / `stop` / `kill`（O2–O4）は絶対に行わない。
     - 終了時は必ず明示プロジェクト名で `docker compose -p alt-verify-<run_id> down -v` を実行して隔離リソースだけを解放する。

## 3. Run

基本コマンド（薄いラッパ `./scripts/verify/verify-live.sh` は `./scripts/altharness/alt-harness verify-live` へ委譲する）:

```bash
# 1. 事前診断（隔離チェック・ツール確認・Feature Map 索引衛生検査）
./scripts/verify/verify-live.sh doctor

# 2. 全ライブプローブ実行（Mode A: 127.0.0.1:0 隔離実行 + 証拠保存）
./scripts/verify/verify-live.sh run --task <task-id> --probe all

# 3. 単一プローブ実行（sse | body-limit | auth-hub | inference-proxy）
./scripts/verify/verify-live.sh run --task <task-id> --probe sse --check-c07-mutation

# 4. C-07 コーパス（before=PASS / after=FAIL）のライブ SSE 検出検証
./scripts/verify/verify-live.sh check-c07-corpus \
  --before scripts/harness/corpus/C-07/before.py \
  --after scripts/harness/corpus/C-07/after.py
```

- `--task <task-id>` は `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$` のみ受理する（`..`・`/`・空白等のパストラバーサルは CWE-22 対策として拒否）。
- `--format json` を渡すと機械可読 JSON を stdout に出力する。

## 4. Doctor

`verify-live run` はプローブ起動前に必ず `doctor` を実行し、1 項目でも失敗すれば fail-closed で停止する。

`doctor` が検証する項目:
1. **Isolation**: `COMPOSE_PROJECT_NAME`（および `--project`）が `alt` でないこと。
2. **Toolchain**: `go`、および `news_creator.handler.sse_stream` を import できる Python（`news-creator/app/.venv/bin/python`、または news-creator の依存が入った `python3` / `uv`）が実行可能であること。`fastapi`+`uvicorn` だけでは足りない。無ければ `news-creator/app` で `uv sync` する。
3. **Evidence Dir**: `.harness/evidence`（または `--evidence-dir`）が安全に書き込み可能であること。
4. **Skills & Symlinks**: `.claude/skills/verify-alt/SKILL.md`、`.claude/skills/maintain-verification-skill/SKILL.md`、および `.agents/skills/{verify-alt,maintain-verification-skill}` シンボリックリンクが壊れていないこと。
5. **Feature Map Index Hygiene**: `.claude/skills/verify-alt/features/` 配下の全 `*.md`（`README.md` を除く）が `features/README.md` からリンクされており、かつ `features/README.md` のリンク先がすべて実在し、必須 4 セクション（`Sub-features` / `How to get to it` / `Driving it with the harness` / `Gotchas`）を備えていること（孤児ファイル 0・リンク切れ 0）。

## 5. Drive

対象機能ごとの詳しい起動・リクエスト手順と確認項目は、対応する Feature Doc に従って駆動する:
- ストリーミング SSE 終端保証 → [features/summarize-sse.md](features/summarize-sse.md)
- Pre-processor BodyLimit 境界 → [features/body-limit.md](features/body-limit.md)
- Auth Hub セッション検証・ヘッダ発行 → [features/auth-hub-session.md](features/auth-hub-session.md)
- Inference Proxy 上流ヘッダ転送 → [features/inference-proxy-headers.md](features/inference-proxy-headers.md)
- Frontend ルート・ナビゲーション面 → [features/frontend-surfaces.md](features/frontend-surfaces.md)

## 6. Observe / Evidence

実行結果は `.harness/evidence/<task>/`（`.gitignore` 対象）に以下の構造で保存され、クリーンアップ後も保持される:

```text
.harness/evidence/<task>/
├── doctor.json           # doctor 検査結果（隔離状態・ツール・索引衛生）
├── summary.json          # 全プローブ合否・所要時間・git HEAD・証拠パス一覧
├── sse.json              # sse プローブ構造化判定（ステータス・ヘッダ・終端確認）
├── sse.log               # sse 実サーバー・実 HTTP レスポンス生ログ
├── body-limit.json       # 1,000,000 vs 1,000,001 バイト判定
├── body-limit.log
├── auth-hub.json         # /health, /validate(401/200), /session(200) 判定
├── auth-hub.log
├── inference-proxy.json  # 6 ヘッダ + auth 除去 + 401/403 判定
└── inference-proxy.log
```

完了報告では必ず実行コマンド・終了コード・`.harness/evidence/<task>/summary.json` のパスと観測値を引用すること。

## 7. Cleanup

- **Mode A**: ハーネス自身が `defer` で全子プロセス（uvicorn / Go overlay サーバー）を停止（stdin EOF → タイムアウト時 SIGKILL）し、すべてのローカルスタブと一時ディレクトリを削除する。`.harness/evidence/<task>/` のみを残す。
- **Mode B**: 検証完了後に必ず `docker compose -p alt-verify-<run_id> down -v` を実行し、`docker compose -p alt-verify-<run_id> ps` が空であることを確認する。`.harness/evidence/<task>/` は残す。

## 8. Maintenance Rule

- ルート・エンドポイント・ヘッダ契約・BodyLimit・SSE ストリーミング・認証挙動を変更する PR は、**同一変更内で**対応する `.claude/skills/verify-alt/features/*.md` を更新する。
- 新機能を追加した場合は `features/<feature>.md` を追加し、`features/README.md` の索引表へリンクを登録する（未登録の孤児ファイルは `verify-live doctor` が検知して落とす）。
- 定期棚卸しやコードとの乖離修復には [.claude/skills/maintain-verification-skill/SKILL.md](../maintain-verification-skill/SKILL.md) を使用する。

## Prior-Art Attribution

本スキルと 4 セクション Feature Map 構造は、Alt 固有の不変条件（同一ホスト `-p alt` 本番隔離、ADR-001028 純 Go 標準ライブラリハーネス、C-07/C-14/C-15 回帰防止）を起点に設計しつつ、Verification Skill / Feature Map / Prove-It-Works の運用様式について `cursor/plugins` の `pstack`（MIT License、一次資料: <https://github.com/cursor/plugins/blob/main/pstack/docs/guide/README.md>）を参照したクリーンルーム実装である。
