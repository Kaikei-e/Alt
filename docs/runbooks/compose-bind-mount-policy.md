---
title: Compose bind mount policy — file-scoped binds, configs, create_host_path
type: runbook
affected_services:
  - compose
owner: platform
date: 2026-08-18
last_updated: 2026-08-18
tags:
  - runbook
  - compose
  - bind-mount
  - alt
---

# Compose bind mount policy

Production compose に **新しい host ファイルを載せるときの決まり**。CI 契約は
`python3 scripts/compose-file-bind-audit.py` が **exit 0** であること
(`.github/workflows/compose-audit.yaml` の `file-bind-invariant`)。

関連: [[PM-2026-036]] [[PM-2026-037]] [[000825]] [[000826]] [[000979]]
[[compose-p2-wave0-feature-gate-2026-08-18]]

## TL;DR

| 載せたいもの | 使う仕組み |
|---|---|
| リポジトリ内の静的設定 (`prometheus.yml`, `postgresql.conf`, ClickHouse XML, …) | Compose トップレベル `configs:` + サービスの `configs:` |
| ホスト専用ファイル (restic SSH 鍵, `docker.sock`) | long-syntax bind + `bind.create_host_path: false` + 原則 `read_only: true` |
| joblib / ML artefact ディレクトリ | directory bind + `create_host_path: false`。**個別ファイル bind は禁止** |

短形式 `host:container:ro` の **ファイル bind は禁止**。短い書き方では
`create_host_path: false` を付けられず、欠落した source が空ディレクトリになる
([[PM-2026-036]])。

`pre_start` は Compose **v5.5.0** で実機 green
(`scripts/compose-feature-gate.report.json` の `pre_start.ok`)。
Wave 1b は chown-only の 2 本
(`news-creator-backend`, `knowledge-embedder-local`) を `pre_start` に移した。
Wave 4 は 14 親の cert ディレクトリ chown を同じく消費者 `pre_start` に置く
（合計 **16** 本の `pre_start`）。
残りの one-shot（migrator / bootstrap / oauth-token-init）は allowlist のまま。

## なぜこの政策があるか

Docker は **存在しないファイル** を短形式 bind すると、warning なしに空
ディレクトリを作ってマウントする。`Path.exists()` は True のまま
`joblib.load()` が `IsADirectoryError` で死に、recap-subworker は 8 日間
healthy のまま分類を返さなかった ([[PM-2026-036]])。

Wave 0 の実機測定 (`docs/review/compose-p2-wave0-feature-gate-2026-08-18.md`)
は Compose v5.0.0 で `pre_start` を reject した。**再測定は v5.5.0**:
`pre_start.ok=true` かつ `create_host_path_false.ok=true`。

- `create_host_path: false` は **効く**。欠落ファイルも欠落ディレクトリも
  daemon が refuse し、空ディレクトリは作られない
- フラグ無しの long-syntax は短形式と同じく空ディレクトリを作る。
  **構文だけ long にしても足場は残る**

named volume + init コンテナ (`service_completed_successfully`) は
rolling `--no-deps` で init が走らず、空 volume のまま本サービスが起動する
([[PM-2026-037]] [[000826]])。artefact の fail-closed を init に預けない。

## 決定ツリー

新しいマウントを足す前に、上から最初に当たった行を使う。

1. **中身が git 管理の静的ファイルか?** → `configs:`
2. **ホストにしか無いファイルか?** (secret 鍵、unix socket) → long-syntax bind +
   `create_host_path: false` + 原則 `read_only: true`
3. **ディレクトリ全体か?** (joblib 置き場、in-repo の `rules/` など)
   - artefact / 欠落したら起動させたくない → long-syntax + `create_host_path: false`
   - チェックアウトすれば必ずある in-repo ディレクトリ → 短形式の directory bind 可
     (監査の対象外。ファイル bind にしないこと)
4. **個別の `*.joblib` / `*.pkl` をファイル bind したくなった** → しない。
   ディレクトリ bind + アプリの `Path.is_file()` validator ([[000825]])

### `configs:` の書き方

```yaml
configs:
  prometheus_yml:
    file: ../observability/prometheus/prometheus.yml

services:
  prometheus:
    configs:
      - source: prometheus_yml
        target: /etc/prometheus/prometheus.yml
        mode: 0444
```

実行ビットが要るラッパは `mode: 0555`。source ファイルが無ければ `up` が
fail-fast する (`secrets:` と同じ)。共有ファイル (postgres `pg_hba.conf` など) は
`compose/base.yaml` に一度だけ定義し、各サービスが参照する。

### long-syntax bind の書き方

```yaml
volumes:
  - type: bind
    source: /var/run/docker.sock
    target: /var/run/docker.sock
    read_only: true
    bind:
      create_host_path: false
```

`create_host_path` を省略すると Engine は欠落 source をディレクトリとして作る。
監査はそれを violation にする。

### Kratos entrypoint

`compose/auth.yaml` は `../kratos:/etc/config/kratos:ro` の directory bind を
既に持っている。`entrypoint.sh` のファイル bind は冗長なので置かない。
entrypoint は `/etc/config/kratos/entrypoint.sh` を指す。

## CI 契約

```bash
python3 scripts/tests/test-compose-file-bind-audit.py   # パーサ + 本番 0 件
python3 scripts/compose-file-bind-audit.py              # 本番 0 件で exit 0
docker compose -f compose/compose.yaml config -q
```

監査が見るもの:

- 短形式のファイル bind (拡張子・SSH 鍵名・実在ファイル)
- 短形式 / フラグ無し long の `docker.sock`
- 短形式 / フラグ無し long の recap artefact ディレクトリ
  (`RECAP_SUBWORKER_DATA_HOST_PATH`, `learning_machine/artifacts`)
- long-syntax ファイル bind で `create_host_path: false` が無いもの

`configs:` は `volumes:` ではないので監査対象外 — それが推奨経路である理由。

`--expect-violations` は Wave 0 の検出器武装用。本番が 0 件の今は **使わない**。
付けると「債務が消えたのにフラグが残っている」として exit 1 になる。

## やってはいけないこと

- 短形式のファイル bind を戻す (`prometheus.yml:/etc/prometheus/prometheus.yml:ro` など)
- long-syntax にして `create_host_path: false` を付け忘れる
- recap-subworker に `*.joblib` のファイル bind を再導入する
- artefact の存在確認を `service_completed_successfully` の init に載せる
  (rolling `--no-deps` と両立しない。chown-only は `pre_start`)

## 欠落時に起きること

| 手段 | source が無いとき |
|---|---|
| 短形式ファイル bind | 空ディレクトリを作成して `up` 成功 (禁止クラス) |
| long-syntax、フラグ無し | 同上 |
| long-syntax + `create_host_path: false` | daemon が refuse。`bind source path does not exist` |
| `configs:` / `secrets:` | `up` が fail-fast |

## Compose ≥ 5.5 / Wave 1b の残り

- **済**: `news-creator-volume-init` → `pre_start` on `news-creator-backend`
- **済**: `knowledge-embedder-local-volume-init` → `pre_start` on
  `knowledge-embedder-local`
- 残る one-shot `service_completed_successfully` は **22** 本
  (`scripts/ops-surface-baseline.json` の init-edge allowlist)。
  Atlas/DB migrator, `step-ca-bootstrap`, `oauth-token-init`,
  `clickhouse-migrator` は Wave 1b の対象外
- 汎用 directory bind 全体への `create_host_path: false` 強制
  (Wave 1 は artefact / ソケット / ファイルに限定)
- `compose/compose.staging.yaml` の Kratos ファイル bind
  (本番 include チェーン外。監査対象外)
