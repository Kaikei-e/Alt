---
name: docker-compose
description: Alt スタックを Docker Compose で操作する。サービスの起動・停止・再ビルド・ログ確認・health check、18 個の stack ファイルに分割された構成のどこにどのサービスが居るかの引き当て、起動しない/変更が反映されない/ポート衝突の切り分けを扱う。ユーザが「起動して」「ログ見せて」「再ビルドして」「立ち上がらない」のようにコンテナ操作を求めたとき、または compose 配下の yaml を編集するときに使う。
allowed-tools: Bash, Read, Glob, Grep
argument-hint: "[service] [up|down|logs|ps|restart]"
paths:
  - "compose/**"
---

# Docker Compose Operations

Alt は Compose を single source of truth とする。K8s は使わない。

## 基本コマンド

`-f compose/compose.yaml -p alt` は常に付ける。省略すると別プロジェクト扱いになり既存コンテナを見失う。

```bash
docker compose -f compose/compose.yaml -p alt up -d                    # 全体
docker compose -f compose/compose.yaml -p alt up -d <service>          # 単体
docker compose -f compose/compose.yaml -p alt up --build -d <service>  # コード変更後
docker compose -f compose/compose.yaml -p alt logs <service> -f
docker compose -f compose/compose.yaml -p alt ps
docker compose -f compose/compose.yaml -p alt down
```

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

## Health check

```bash
curl http://localhost/health          # Frontend (nginx 経由)
curl http://localhost:9000/v1/health  # alt-backend
curl http://localhost:9250/health     # BFF (alt-butterfly-facade)
curl http://localhost:7700/health     # Meilisearch
```

## Gotchas

環境固有で、素直に推測すると外す点。

- **`restart` は host port を再 bind しない。** bind に失敗したコンテナは `--force-recreate` が必要で、`restart` では直らない
- **`network_mode: service:` の pki-agent サイドカーは親と同時に recreate する。** 親だけ再作成すると相乗りサイドカーが孤立し、無言で壊れる。`--force-recreate <親> pki-agent-<親>` と並べて指定する
- **Go / Rust / TypeScript の変更は `--build` が必須。** 付け忘れると古いバイナリが動き続け、「変更が反映されない」の原因になる
- **起動しないときは `down` してから `up -d`。** 中途半端な状態からの `up` は依存解決に失敗しやすい
- ポート衝突は `docker ps` で占有コンテナを先に特定する
