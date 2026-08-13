---
name: docker-compose
description: Alt スタックを Docker Compose で操作する。18 個の stack ファイルのどこにどのサービスが定義されているかの引き当てと、再ビルド・強制再作成・サイドカーの同時再作成のような素直に推測すると外す操作を扱う。ユーザが「起動して」「立ち上げて」「再ビルドして」「落として」「立ち上がらない」「反映されない」のようにコンテナ操作を求めたとき、または `compose/` 配下の yaml を編集するときに使う。障害の原因をログや DB から掘るなら log-seeker を使う（このスキルはコンテナ操作と構成の引き当て担当）。
allowed-tools: Bash, Read, Glob, Grep
paths:
  - "compose/**"
---

# Docker Compose Operations

基本の `up -d` / `logs` / `down` と health check の curl は CLAUDE.md にある。ここはその先。

## CLAUDE.md にないコマンド

```bash
docker compose -f compose/compose.yaml -p alt up --build -d <service>          # コード変更後
docker compose -f compose/compose.yaml -p alt up -d --force-recreate <service> # bind し直し・sidecar 復旧
docker compose -f compose/compose.yaml -p alt ps
```

`-p alt` を省略すると別プロジェクト扱いになり、既存コンテナを見失う。

## スタック構成（Compose profiles ではなく `include:`）

`compose/compose.yaml` が 18 個の stack ファイルを `include:` する。profile ではないので `--profile` は使わない。
サービス名からどのファイルを編集すべきかを引く表:

| Stack file | Services |
|---|---|
| `db.yaml` | db, meilisearch, clickhouse, pre-processor-db, pre-processor-db-migrator |
| `core.yaml` | plecto-proxy, alt-frontend-sv, alt-backend, migrate |
| `bff.yaml` | alt-butterfly-facade |
| `auth.yaml` | kratos-db, kratos-migrate, kratos, auth-hub |
| `workers.yaml` | pre-processor-sidecar, search-indexer, tag-generator, oauth-token-init, auth-token-manager |
| `ai.yaml` | redis-cache, news-creator-backend, news-creator, news-creator-volume-init, pre-processor |
| `mq.yaml` | redis-streams, mq-hub |
| `rag.yaml` | rag-db, rag-db-migrator, rag-orchestrator, knowledge-embedder-local, rerank-local |
| `recap.yaml` | recap-db, recap-db-migrator, recap-worker, recap-subworker, dashboard, recap-evaluator |
| `acolyte.yaml` | acolyte-db, acolyte-db-migrator, acolyte-orchestrator |
| `sovereign.yaml` | knowledge-sovereign-db, knowledge-sovereign-db-migrator, knowledge-sovereign |
| `logging.yaml` | rask-log-aggregator と各サービスの `*-logs` forwarder |
| `observability.yaml` | prometheus, grafana, cadvisor |
| `pki.yaml` | step-ca, step-ca-bootstrap, `pki-agent-<親サービス>` 群 |
| `pgbouncer.yaml` | pgbouncer, pgbouncer-kratos |
| `pact.yaml` | pact-db, pact-broker |
| `perf.yaml` | alt-perf, k6 |
| `backup.yaml` | docker-socket-proxy, restic-backup |

## Gotchas

環境固有で、素直に推測すると外す点。

- **`restart` は host port を再 bind しない。** bind に失敗したコンテナは `--force-recreate` が必要で、`restart` では直らない
- **`network_mode: service:` の pki-agent サイドカーは親と同時に recreate する。** 親だけ再作成すると相乗りサイドカーが孤立し、無言で壊れる。`--force-recreate <親> pki-agent-<親>` と並べて指定する
- **起動しないときは `down` してから `up -d`。** 中途半端な状態からの `up` は依存解決に失敗しやすい
- ポート衝突は `docker ps` で占有コンテナを先に特定する
