# Feed Read 3000VU Load Test

Connect-RPC (v2) の読み取り系 API に対して 3000VU の同時閲覧負荷を再現する k6 負荷試験。

## 前提

- Alt スタックが起動済み (`docker compose -f compose/compose.yaml -p alt up -d`)
- Docker secret `backend_token_secret` が設定済み
- PostgreSQL にフィード・記事データが存在すること

## API 仕様の想定

すべて Connect-RPC v2 (POST + JSON body)。**全エンドポイントが DB 読み取りのみ**で、外部 URL フェッチは一切行わない。

| 操作 | エンドポイント | 備考 |
|------|---------------|------|
| フィード一覧 | `POST /alt.feeds.v2.FeedService/GetUnreadFeeds` | cursor-based pagination |
| フィード記事一覧 | `POST /alt.feeds.v2.FeedService/GetAllFeeds` | cursor-based pagination |
| 個別記事詳細 | `POST /alt.articles.v2.ArticleService/FetchArticlesCursor` | DB-only (NOT FetchArticleContent) |
| 未読カウント | `POST /alt.feeds.v2.FeedService/GetUnreadCount` | |

**注**: `FetchArticleContent` は外部 URL を取得するため SSRF AllowedList 問題が発生する。
読み取り負荷試験では `FetchArticlesCursor` を代わりに使用する。

## 認証

Connect-RPC (port 9101) は JWT 認証 (`X-Alt-Backend-Token`) を使用する。
REST (port 9000) の `X-Alt-Shared-Secret` とは**異なる認証方式**。

| 項目 | 値 | 備考 |
|------|-----|------|
| ヘッダー | `X-Alt-Backend-Token` | JWT トークン |
| 署名方式 | HMAC-SHA256 | `BACKEND_TOKEN_SECRET` で署名 |
| Issuer | `auth-hub` | デフォルト値 |
| Audience | `alt-backend` | デフォルト値 |
| Subject | `users.sample.json` の `user_id` | **有効な UUID** (`uuid.Parse()` 必須) |
| Claims | `email`, `role`, `sid` | ユーザー属性 |

JWT は `k6/helpers/jwt.js` で k6 内部で生成される。

## 環境変数

| 変数 | デフォルト | 説明 |
|------|-----------|------|
| `K6_BASE_URL` | `http://alt-backend:9101` | Connect-RPC URL (実行スクリプトが自動設定) |
| `K6_BACKEND_TOKEN_SECRET` | Docker secret から自動注入 | JWT 署名用秘密鍵 |
| `USERS_FILE` | `/scripts/data/users.sample.json` | ユーザーデータパス |
| `FEEDS_FILE` | `/scripts/data/feeds.sample.json` | フィードデータパス |

## 実行方法

### 推奨: 実行スクリプト経由

実行スクリプトが DoS Protection/リソースの環境変数オーバーライドを自動設定する。

```bash
# フルテスト (30分, 3000VU)
./alt-perf/scripts/run-feed-read-load-test.sh

# Smoke test (1VU, 5イテレーション)
VU_COUNT=1 ./alt-perf/scripts/run-feed-read-load-test.sh

# 中規模テスト
VU_COUNT=100 ./alt-perf/scripts/run-feed-read-load-test.sh
```

### 手動実行 (非推奨)

DoS Protection の引き上げなしで実行すると、即座にレートリミットに引っかかる。

```bash
docker compose -f compose/compose.yaml -f compose/perf.yaml -p alt \
  run --rm k6 run /scripts/scenarios/feed-read-3000vu.js
```

## 実行スクリプトが行うこと

`run-feed-read-load-test.sh` は以下の環境変数オーバーライドを自動適用する:

| 設定 | デフォルト値 | テスト時の値 | 理由 |
|------|------------|------------|------|
| `DOS_PROTECTION_RATE_LIMIT` | 100 | 10000 | 3000VU のリクエストを許容 |
| `DOS_PROTECTION_BURST_LIMIT` | 200 | 20000 | バースト対応 |
| `DB_MAX_CONNS` | (default) | 200 | 高並列 DB 接続 |
| k6 メモリ | 512MB | 16GB | 3000VU のメモリ要件 |
| PG max_connections | 100 | 250 | 高並列 DB 接続 |
| PgBouncer MAX_CLIENT_CONN | (default) | 2000 | 接続プーリング |

テスト後は自動的にデフォルト設定に復元される。

## 結果の見方

### 閾値

| メトリクス | 閾値 | 説明 |
|-----------|------|------|
| `http_req_failed` | < 1% | 全体エラー率 |
| `http_req_duration` p95 | < 1500ms | 全体レイテンシ |
| `http_req_duration` p99 | < 3000ms | 全体テールレイテンシ |
| `feed_list_duration` p95 | < 800ms | フィード一覧 |
| `feed_items_duration` p95 | < 1200ms | 記事一覧 |
| `item_detail_duration` p95 | < 1000ms | 記事詳細 |

### カスタムメトリクス

| メトリクス | 説明 |
|-----------|------|
| `feed_list_duration` | フィード一覧 API のレスポンスタイム |
| `feed_items_duration` | 記事一覧 API のレスポンスタイム |
| `item_detail_duration` | 記事詳細 API のレスポンスタイム |
| `rate_limit_hits` | 429 レスポンスのカウント (DoS Protection) |
| `auth_errors` | 401/403 レスポンスのカウント |
| `server_errors` | 5xx レスポンスのカウント |

### レポート

`reports/k6-*.json` に JSON レポートが出力される。

## URL Grouping

Connect-RPC パスは固定なので高カーディナリティ問題は発生しない。
各リクエストに `tags: { name: "feed-list" }` 等の name tag を付与しており、
k6 のサマリーで操作別のレイテンシを確認できる。

## データ分布 (80/20 アクセスパターン)

テストデータは IMPL.md の「人気フィード200件、通常フィード800件、80/20 アクセス偏り」要件を実データの76 feed_links に比例適用する。

### フィード分類

`ntile(5)` でフィードをフィード数 (= 記事数) 降順に5分位に分割し、上位20% を「人気」、残り80% を「通常」に分類する:

```sql
SELECT fl.id, count(f.id) AS feed_count,
  CASE WHEN ntile(5) OVER (ORDER BY count(f.id) DESC) = 1
       THEN 'popular' ELSE 'normal' END AS tier
FROM feed_links fl
LEFT JOIN feeds f ON f.feed_link_id = fl.id
GROUP BY fl.id;
```

76 feed_links の場合: 人気 ~15件 / 通常 ~61件

### 購読パターン

| 区分 | 購読ルール | 3000VU での見込み |
|------|-----------|-----------------|
| 人気フィード (20%) | **全ユーザーが購読** | 3000 × 15 = 45,000 subscriptions |
| 通常フィード (80%) | **各ユーザーが 30% をランダム購読** | 3000 × 61 × 0.3 ≈ 54,900 subscriptions |

### 記事コピー

| 区分 | 記事数/ユーザー | 理由 |
|------|---------------|------|
| 人気フィード | 15 | 高頻度アクセス対象に多めの記事 |
| 通常フィード | 5 | 低頻度アクセス対象に少なめの記事 |

### なぜ 80/20 が実現されるか

- 人気フィードは **100% のユーザー** の `GetUnreadFeeds` 結果に含まれる
- 通常フィードは **~30% のユーザー** の結果にのみ含まれる
- 人気フィードの記事数が多い (15 vs 5) ため、DB 読み取り負荷は人気フィードに集中する

## 行動分布

| シナリオ | 割合 | フロー |
|---------|------|--------|
| glance | 10% | feed-list のみ |
| browse | 55% | feed-list → feed-items |
| paginate | 20% | feed-list → feed-items → feed-items (cursor) |
| deep-read | 15% | feed-list → feed-items → item-detail (ArticlesCursor) |

## よくある失敗例

### `auth_errors` が大量に出る

- `backend_token_secret` が Docker secret として存在するか確認: `docker secret ls` またはファイル `secrets/backend_token_secret.txt`
- `users.sample.json` の `user_id` が有効な UUID か確認 (auth interceptor が `uuid.Parse()` する)
- alt-backend の `BACKEND_TOKEN_SECRET` と k6 の `K6_BACKEND_TOKEN_SECRET` が同じ値か確認
- **注意**: Connect-RPC (port 9101) は REST (port 9000) とは異なる認証方式を使用する

### `rate_limit_hits` が多い

- **実行スクリプト (`run-feed-read-load-test.sh`) 経由で実行すること**
- 手動実行の場合、DoS Protection のデフォルト (100 req/min/IP) では 3000VU に耐えられない
- 前回テスト (feed-registration) では DoS 10000 に引き上げて解消

### k6 自体がボトルネックになる

- `htop` で k6 プロセスの CPU/メモリ使用率を監視
- 実行スクリプトは k6 コンテナに 16GB / 8CPU を割り当てる
- `discardResponseBodies: true` が有効 (レスポンスボディをメモリに保持しない)

### socket: too many open files

- `ulimit -n 65535` を設定してから実行
- Docker の場合は実行スクリプトが対処済み

## 3000VU 実行時の注意点

1. **実行スクリプト必須**: DoS Protection/リソースの環境変数オーバーライドなしでは機能しない
2. **段階的実行**: いきなり 3000VU ではなく、`VU_COUNT=1` → `100` → `3000` と段階的に実行
3. **OS チューニング**: `ulimit -n` を 65535 以上に設定
4. **CPU/メモリ監視**: k6 実行マシン自体がボトルネックにならないよう監視
5. **DB コネクション**: PostgreSQL の `max_connections` が十分か確認 (スクリプトが 250 に設定)
6. **テスト後の復元**: 実行スクリプトがデフォルト設定に自動復元するが、中断した場合は手動で復元すること
   ```bash
   docker compose -f compose/compose.yaml -p alt up -d --force-recreate alt-backend db pgbouncer
   rm -f compose/feed-read-test-generated.yaml
   ```
