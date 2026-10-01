# kratos-db

_Last reviewed: September 5, 2026_

**Location:** `kratos-db/`
**Port:** 5434 (host, bound to `127.0.0.1`) → 5432 (container)

## Role
- Ory Kratos 専用の PostgreSQL 16 データベース (`postgres:16.15-alpine`)
- ユーザーアイデンティティ、認証クレデンシャル、セッションデータの永続化ストア

## Architecture & Compose Integration

| Property | Value |
| --- | --- |
| Image | `postgres:16.15-alpine` (`compose/auth.yaml`) |
| Container Port | 5432 |
| Host Port | `127.0.0.1:5434:5432` |
| Volume | `kratos_db_data` (`/var/lib/postgresql/data`) |
| Secrets | `kratos_db_password` |
| Database | `kratos` (環境変数 `POSTGRES_DB` / `KRATOS_DB_NAME`) |
| User | `kratos_user` (環境変数 `POSTGRES_USER` / `KRATOS_DB_USER`) |
| Init Directory | `../kratos-db/init` (`/docker-entrypoint-initdb.d:ro`) |
| Configs | `postgres_postgresql_conf`, `postgres_pg_hba_conf` |

## Access Paths & Connection Pooling

- **`kratos` サービス (常駐)**: 直接 `kratos-db` には接続せず、接続プーラー `pgbouncer-kratos` (port 6432, transaction pooling) を経由する。
- **`kratos-migrate` (ワンショット)**: DDL の実行およびセッションレベルのロック取得が必要なため、PgBouncer をバイパスして直接 `kratos-db:5432` に接続する。
- **外部サービス**: 他の Alt サービスからの直接 SQL 接続は禁止されており、アイデンティティへのアクセスはすべて Kratos / [[auth-hub]] の API 経由で行われる。

## Schema Management & Migrations

- スキーマは Ory Kratos 公式のマイグレーションツール (`kratos migrate sql`) によって管理される。
- Alt monorepo 側で手動 DDL や Atlas による改変は一切行わず、Kratos のイメージバージョン更新に合わせてマイグレーションを適用する。

## Health Check & Runbook

```bash
# PostgreSQL readiness
pg_isready -h localhost -p 5434 -U kratos_user -d kratos

# 接続確認
psql -h localhost -p 5434 -U kratos_user -d kratos -c "SELECT 1"

# サービス再起動
docker compose -f compose/auth.yaml up -d kratos-db
```

## Known Failure Patterns & Invariants

Cross-cutting incident patterns are catalogued in [[runbooks/crystallized-knowledge]].

- **DDL は必ず PgBouncer をバイパスする**: PgBouncer のトランザクションプーリング経由でマイグレーションを実行すると、DDL ロックやセッション変数のスコープ不整合によりマイグレーションがハングまたは失敗する ([[000327]])。
- **Docker shm_size**: PostgreSQL コンテナはクエリ負荷時に work_mem 用の共有メモリを消費するため、必要に応じて shm_size の枯渇に留意する。
