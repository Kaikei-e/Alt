# redis-cache

_Last reviewed: October 3, 2026_

Redis 8.4.5-alpine (`--maxmemory 256mb --maxmemory-policy allkeys-lru`)。LLM response cache。

## ポート

- コンテナ内 6379（`alt-network` 内部専用、ホスト未公開）

## Health

- `REDISCLI_AUTH="$(cat /run/secrets/redis_cache_password)" redis-cli --user cache ping | grep -q PONG`

## ACL

`docker/redis/entrypoint.sh` が生成する ACL で `user default off`。認証なしの `redis-cli` は `ping` も含めて `NOAUTH` になる。唯一のユーザ `cache` はキー `recap_card:*` / `recap:summary:*` に対する `PING` / `GET` / `SET` / `DEL` だけを持ち、`INFO` / `SCAN` / `TTL` は `NOPERM`。運用調査で読めるのは既知キーの `GET` のみ。

## Volume

- `redis-cache-data:/data`

## Secrets

- `redis_cache_password` (`cache` ユーザ)

## 主要利用者

- `news-creator` (要約結果のキャッシュ、`CACHE_REDIS_URL=redis://cache@redis-cache:6379/0` + `REDIS_PASSWORD_FILE=/run/secrets/redis_cache_password`)

## 設計原則 & 注意

- cache key は決定的 (同入力 → 同 key)
- cache miss を許容する設計に保つ
- メモリポリシーは `allkeys-lru` で、キャッシュ容量（256MB）到達時は LRU 破棄される
