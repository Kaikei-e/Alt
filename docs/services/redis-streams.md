# redis-streams

_Last reviewed: October 3, 2026_

Redis 8.4.5 (`--maxmemory 1gb --maxmemory-policy noeviction`)。イベントストリームの **backbone**。

## ポート

- 6380 (host。コンテナ内部は 6379)

## Health

- `REDISCLI_AUTH="$(cat /run/secrets/redis_streams_password)" redis-cli --user streams ping | grep -q PONG`

## ACL

`docker/redis/entrypoint.sh` が起動時に secret のハッシュから ACL ファイルを生成する。`user default off` のため、認証なしの `redis-cli` は `ping` も含めて `NOAUTH` になる。

| ユーザ | キー | 用途 |
| --- | --- | --- |
| `streams` | `alt:events:*`, `alt:replies:tags:*` | mq-hub / 各コンシューマーのストリーム操作 (`XADD` / `XREADGROUP` / `XACK` / `XAUTOCLAIM` / `XPENDING` / `XINFO STREAM` / `XINFO GROUPS` / `XTRIM` / `SCAN` 等) |
| `limiter` | `host_rate_limiter:v1:*` | alt-backend と alt-harvester の host rate limiter (DB 3、`SET` / `PTTL` / `SELECT`) |

`INFO` / `XLEN` / `XRANGE` / `TYPE` / `DBSIZE` / `XINFO CONSUMERS` はどのユーザでも `NOPERM`。運用調査ではストリーム長を `XINFO STREAM` の `length`、滞留を `XINFO GROUPS` の `pending` / `lag` から読む:

```bash
docker compose -f compose/compose.yaml -p alt exec -T redis-streams sh -c \
  'REDISCLI_AUTH="$(cat /run/secrets/redis_streams_password)" redis-cli --user streams XINFO GROUPS <stream-key>'
```

## Volume

- `redis-streams-data:/data`

## Secrets

- `redis_streams_password` (`streams` ユーザ)
- `redis_limiter_password` (`limiter` ユーザ)

## 主要利用者

- `mq-hub`: publish (`XAdd`) と consumer group 作成のみ。`XREADGROUP` は呼ばない
- `pre-processor`, `search-indexer`, `tag-generator`: `REDIS_STREAMS_URL` で直接接続し `XREADGROUP` + `XAUTOCLAIM` で consume。業務イベントの publish は mq-hub の Connect-RPC 経由だが、DLQ への `XADD` (配信回数超過メッセージの退避) は各サービスが自分の DLQ ストリームへ直接行う
- `alt-backend`: event の publish は mq-hub の Connect-RPC 経由。DB を分けて (`/3`) rate limiter のバックエンドとしても直接接続する ([[mq-hub]] とは独立した用途)
- `alt-harvester`: alt-backend と同じ host rate limiter に `limiter` ユーザで `/3` へ直接接続する (`HOST_RATE_LIMITER_REDIS_URL=redis://limiter@redis-streams:6379/3`)。インターネットに出る 2 バイナリで同一ホストへの最小間隔を 1 つの arbiter に保つため

## 設計原則

- consumer group + ack (書き込みが durable になった後にのみ ACK) で at-least-once
- DLQ は各コンシューマー自身が実装する (`XAUTOCLAIM` で配信回数が閾値を超えたメッセージを専用ストリームへ `XADD`)。mq-hub は DLQ を書かないが、周期 `XTRIM` で全ストリームの絶対上限を維持する。ただし tag-generator の `alt:events:tags` 専用コンシューマーが作る `alt:events:tags:dlq` は mq-hub の周期トリム対象リストに含まれず、自身の `XADD MAXLEN` だけが上限になる
- メモリ retention は明示設定 (mem 圧迫を起こさない)。8.4.0〜8.4.3 系は `MAXLEN ~` を `ACKED` トリム戦略と組み合わせると削除が効かない不具合があるため 8.4.4 以降に固定する
