# pre-processor-db

_Last reviewed: September 5, 2026_

PostgreSQL 17。pre-processor / pre-processor-sidecar 専有 DB。feed metadata は持たない（feeds / feed_links は alt-db にあり、alt-data-hub がオーナー）。

## ポート

- 5437 (host `127.0.0.1:5437:5432`)

## Health

- `pg_isready -U ${PP_DB_USER:-pp_user} -d ${PP_DB_NAME:-pre_processor}`

## Volume

- `pre_processor_db_data:/var/lib/postgresql/data`

## テーブル

- `inoreader_subscriptions`, `inoreader_articles`, `sync_state`, `api_usage_tracking` — pre-processor-sidecar が書く Inoreader ステージング系
- `summarize_job_queue` — pre-processor の要約ジョブキュー
- `notification_outbox` — pre-processor から alt-data-hub への通知転送用 outbox（`pre-processor-migration-atlas/migrations/20260808000100_create_notification_outbox.sql`）

## Schema 管理

- `pre-processor-db-migrator`（Atlas、`pre-processor-migration-atlas/`）により起動時にスキーマ適用

## Owner

- pre-processor / pre-processor-sidecar が読み書き（専有 DB、[[000246]]）

## Secrets

- `pp_db_password`
