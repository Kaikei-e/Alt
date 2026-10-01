# redis-cache

_Last reviewed: May 17, 2026_

Redis 8.4.5-alpine (`--maxmemory 256mb --maxmemory-policy allkeys-lru`)。LLM response cache。

## ポート

- コンテナ内 6379（`alt-network` 内部専用、ホスト未公開）

## Health

- `REDISCLI_AUTH="$(cat /run/secrets/redis_password)" redis-cli ping | grep -q PONG`

## Volume

- `redis-cache-data:/data`

## Secrets

- `redis_password`

## 主要利用者

- `news-creator` (要約結果のキャッシュ、`CACHE_REDIS_URL=redis://redis-cache:6379/0`)

## 設計原則 & 注意

- cache key は決定的 (同入力 → 同 key)
- cache miss を許容する設計に保つ
- メモリポリシーは `allkeys-lru` で、キャッシュ容量（256MB）到達時は LRU 破棄される
