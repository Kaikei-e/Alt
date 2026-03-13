# SV Connect-RPC Composite 10m Load Test

`alt-frontend-sv` の実運用経路に合わせて、`/sv/api/v2/*` の Connect-RPC を 10 分間まとめて叩く複合負荷試験です。

- 対象経路: browser 相当の `nginx -> /sv/api/v2 -> SvelteKit proxy -> alt-backend`
- 外部HTTP: **mock-rss-server のみ**
- 目的: 「何 sessions/sec まで閾値内でさばけるか」を測り、そこから同時ユーザー数を推定する

## なぜこのモデルか

- SV フロントの本番入口である `/sv/api/v2` を通すので、SvelteKit proxy・`hooks.server.ts`・auth-hub のコストも含めて測れる
- k6 は `ramping-arrival-rate` を使う open model
  - セッション開始レートを固定できる
  - サーバーが遅くなっても発火レートが下がらず、処理限界を見つけやすい
- 1 iteration = 1 user session とし、内部に think time を入れて「ページ遷移を伴う利用」を表現する

## セッション構成

重みは SV フロントで使われている Connect-RPC クライアントに寄せています。

| Flow | 比率 | Connect-RPC |
|------|------|-------------|
| Browse | 45% | `GetUnreadCount` → `GetUnreadFeeds` → `GetAllFeeds` |
| Read | 25% | `GetUnreadFeeds` → `FetchArticlesCursor` → `FetchArticleContent` → `MarkAsRead` |
| Discovery | 20% | `GetFeedStats` → `GetDetailedFeedStats` → `SearchFeeds` → `GetReadFeeds` → `GetFavoriteFeeds` |
| Manage Feeds | 10% | `ListRSSFeedLinks` → `RegisterRSSFeed` → `RegisterFavoriteFeed` → `ListRSSFeedLinks` |

## 外部リクエストの扱い

外部HTTPを伴うのは次の 2 系統だけです。

- `RegisterRSSFeed`
  - `ValidateAndFetchRSSGateway` が mock RSS を取得
- `FetchArticleContent`
  - mock article HTML を取得

mock サーバーは [`alt-perf/k6/mock-rss-server/main.go`](/home/koko/Documents/dev/Alt/alt-perf/k6/mock-rss-server/main.go) で、RSS/記事HTML/OG画像を返します。遅延は環境変数で調整できます。

## インフラ設定のオーバーライド

実行スクリプトは `compose/sv-connect-load-test-generated.yaml` を生成し、負荷試験に必要な設定を一時的に上書きします。

| サービス | 環境変数 | デフォルト | オーバーライド | 理由 |
|----------|---------|-----------|--------------|------|
| alt-backend | `FEED_ALLOWED_HOSTS` | (空) | `mock-rss-001` | mock-rss-server への SSRF 許可 |
| alt-backend | `DOS_PROTECTION_RATE_LIMIT` | 100 | 5000 | VU 並列リクエストでの 429 回避 |
| alt-backend | `DOS_PROTECTION_BURST_LIMIT` | 200 | 10000 | 同上 |
| alt-backend | `RATE_LIMIT_EXTERNAL_API_INTERVAL` | 10s | 10ms | mock-rss への per-host レート制限緩和 (100 req/s) |
| alt-backend | `RATE_LIMIT_EXTERNAL_API_BURST` | 3 | 500 | mock-rss へのバースト許容量増加 |
| alt-backend | `RATE_LIMIT_EXTERNAL_API_UNSAFE_FAST` | false | true | sub-second interval のバリデーションバイパス（要: alt-backend コード一時変更、後述） |
| auth-hub | `VALIDATE_RATE_LIMIT` | ~1.67 req/s | 1000 req/s | `/validate` 認証検証のボトルネック回避 |
| auth-hub | `SESSION_RATE_LIMIT` | 0.5 req/s (30 req/min) | 1000 req/s | `/session` JWT取得のボトルネック回避 |

### リクエストフロー

```text
k6 VU → nginx (/sv/api/v2/*) → alt-frontend-sv (SvelteKit proxy)
  → Kratos toSession()                            ... セッション検証
  → auth-hub /session (JWT backend token 取得)     ← SESSION_RATE_LIMIT
  → alt-backend:9101 (Connect-RPC)                ← DOS_PROTECTION_*
    → mock-rss-001:8080 (外部HTTP)                ← FEED_ALLOWED_HOSTS
```

### なぜこれらが必要か

1. **FEED_ALLOWED_HOSTS**: alt-backend は SSRF 防止のため、外部 HTTP リクエスト先を `FEED_ALLOWED_HOSTS` でホワイトリスト管理している。mock-rss-server のホスト名 `mock-rss-001` を許可しないと `RegisterRSSFeed` / `FetchArticleContent` が失敗する
2. **DOS_PROTECTION**: デフォルトは 100 req/min。数十 VU が各セッションで 3〜6 回の RPC を叩くと瞬時に超過する。5000 に緩和することで負荷試験中の誤ブロックを防ぐ
3. **RATE_LIMIT_EXTERNAL_API**: `FetchArticleContent` が `HostRateLimiter` (`golang.org/x/time/rate` の token bucket) でホスト単位に制御される。`rate.Every(interval)` が定常レートを決め、`burst` は初期トークン数。デフォルト 10s 間隔（0.1 req/s）・バースト 3 では全 VU が `mock-rss-001` に集中するため即座にタイムアウトする。高負荷試験では interval=10ms（100 req/s）・burst=500 に緩和。**interval < 1s にはバリデーションの 1 秒最小値ガードがあるため、`RATE_LIMIT_EXTERNAL_API_UNSAFE_FAST=true` が必須**
4. **VALIDATE_RATE_LIMIT**: 旧 Next.js パス (`/api/backend/*`) は nginx `auth_request` → auth-hub `/validate` を通過する。デフォルト ~1.67 req/s では負荷試験時に詰まるため緩和
5. **SESSION_RATE_LIMIT**: SV パス (`/sv/api/v2/*`) は SvelteKit `hooks.server.ts` → auth-hub `/session` で JWT を取得する。デフォルト 0.5 req/s (30 req/min) では 2 VU でも即座に 429 が返るため 1000 req/s に緩和

### SSRF バリデータについて

Docker ネットワーク内では `mock-rss-001` がプライベート IP（172.x.x.x）に解決されるため、`SSRFValidator` が DNS rebinding 攻撃と誤検知してブロックします。これを回避するにはコードの一時変更が必要です（「負荷試験時の alt-backend コード一時変更」セクション参照）。

### mock-rss-server の 3 レプリカ化

高負荷試験 (1000 VU+) では mock-rss-server を 3 レプリカに増やし、Docker DNS ラウンドロビンで負荷分散する。

**オーバーレイでの変更点:**

```yaml
mock-rss-server:
  ports: !reset []          # ホストポート 8091:8080 を無効化（レプリカ間でポート競合するため）
  deploy:
    replicas: 3             # 3 レプリカで負荷分散
  networks:
    alt-network:
      aliases:
        - mock-rss-001      # 全レプリカが同一 DNS エイリアスを共有
```

- `ports: !reset []` — ホストポートマッピングを無効化。複数レプリカではポート 8091 が競合するため必須
- ヘルスチェックは `docker compose exec -T mock-rss-server wget --spider -q http://localhost:8080/health` に変更（ホストポート経由が使えないため）
- DNS エイリアス `mock-rss-001` は全レプリカで共有され、Docker 内部 DNS がラウンドロビンで振り分ける

### 負荷試験時の alt-backend コード一時変更

高負荷試験では alt-backend のセキュリティバリデーションを一時的に緩和する必要がある。**これらの変更は本番コードにマージしないこと。試験後に必ず `git checkout -- alt-backend/` で revert する。**

#### 1. レート制限 interval < 1s の許可

`HostRateLimiter` は `rate.Every(interval)` で定常レートを設定する。interval=1s だとバースト消費後は **1 req/s** しか通らない。高負荷試験で interval を 1 秒未満にするには、以下 2 ファイルを一時変更する:

**`alt-backend/app/config/config.go`** — `RateLimitConfig` にフィールドを追加:

```go
type RateLimitConfig struct {
    ExternalAPIInterval  time.Duration `json:"external_api_interval" env:"RATE_LIMIT_EXTERNAL_API_INTERVAL" default:"10s"`
    ExternalAPIBurst     int           `json:"external_api_burst" env:"RATE_LIMIT_EXTERNAL_API_BURST" default:"3"`
    ExternalAPIUnsafeFast bool         `json:"external_api_unsafe_fast" env:"RATE_LIMIT_EXTERNAL_API_UNSAFE_FAST" default:"false"`
    FeedFetchLimit       int           `json:"feed_fetch_limit" env:"RATE_LIMIT_FEED_FETCH_LIMIT" default:"100"`
    // ...
}
```

**`alt-backend/app/config/validation.go`** — バリデーションをバイパス:

```go
// 変更前:
if config.ExternalAPIInterval < time.Second {

// 変更後:
if !config.ExternalAPIUnsafeFast && config.ExternalAPIInterval < time.Second {
```

これにより `RATE_LIMIT_EXTERNAL_API_UNSAFE_FAST=true` 環境変数で 1 秒未満の interval を許可できる。

#### 2. SSRF AllowList による private IP 許可

Docker ネットワーク内では `mock-rss-001` がプライベート IP (172.x.x.x) に解決されるため、`SSRFValidator` が DNS rebinding 攻撃と誤検知する。`FEED_ALLOWED_HOSTS` 環境変数だけでは不十分で、バリデータに IP 承認ロジックが必要:

**`alt-backend/app/utils/security/ssrf_validator.go`** — 3 箇所を変更:

```go
// 1. import に "sync" を追加
import (
    // ...
    "sync"
    // ...
)

// 2. SSRFValidator struct に approvedPrivateIPs フィールドを追加
type SSRFValidator struct {
    // ...既存フィールド...
    approvedPrivateIPs sync.Map // map[string]struct{}
}

// 3. validateResolvedIP() 内の private IP チェックに AllowList バイパスを追加
if !isTestingLocalhost && v.isPrivateOrDangerous(ip) {
    // ↓ この if ブロックを追加
    if IsFeedHostAllowed(hostname) {
        v.approvedPrivateIPs.Store(ip.String(), struct{}{})
        return nil
    }
    return &ValidationError{...}
}

// 4. validateConnectionIP() 内の private IP チェックに approved IP バイパスを追加
if v.isPrivateOrDangerous(ip) {
    // ↓ この if ブロックを追加
    if _, ok := v.approvedPrivateIPs.Load(ip.String()); ok {
        return nil
    }
    return &ValidationError{...}
}
```

**セキュリティ上の保証:**

- クラウドメタデータ IP (169.254.169.254 等) は `isMetadataEndpointIP()` で**常にブロック**される
- `FEED_ALLOWED_HOSTS` が空 (デフォルト) の場合、バイパスは一切発生しない

#### revert 手順

```bash
git checkout -- alt-backend/app/config/config.go alt-backend/app/config/validation.go alt-backend/app/utils/security/ssrf_validator.go
```

### トラブルシューティング

| 症状 | k6 カウンター | 対処 |
|------|-------------|------|
| 429 が多発 | `rate_limit_hits` 増加 | `DOS_PROTECTION_RATE_LIMIT` を上げる |
| 401/403 が多発 | `auth_errors` 増加 | `SESSION_RATE_LIMIT` / `VALIDATE_RATE_LIMIT` を確認、auth-hub ログで 429 を確認 |
| RegisterRSSFeed で 5xx | `server_errors` 増加 | `FEED_ALLOWED_HOSTS` にホスト名が含まれているか確認 |
| login 失敗 | `login_errors` 増加 | Kratos が起動しているか、テストユーザーが作成されたか確認 |

## 10分プロファイル

デフォルトのセッション開始レート:

| 区間 | 長さ | sessions/sec |
|------|------|--------------|
| warm-up | 1m | 2 |
| ramp | 2m | 2 → 5 |
| stable | 4m | 5 |
| ramp | 2m | 5 → 8 |
| peak | 1m | 8 |

調整用の主な env:

- `SESSION_RATE_BASE`
- `SESSION_RATE_TARGET`
- `SESSION_RATE_PEAK`
- `PRE_ALLOCATED_VUS`
- `MAX_VUS`
- `MOCK_RSS_DELAY_MS`
- `MOCK_ARTICLE_DELAY_MS`
- `MOCK_DELAY_JITTER_MS`

## 実行

前提:

- Alt stack が起動可能
- `deno` がローカルにある
- `compose/load-test.yaml` と `compose/perf.yaml` が利用可能

実行:

```bash
./alt-perf/scripts/run-sv-connect-composite-test.sh
```

負荷を上げる例:

```bash
USER_COUNT=400 \
SESSION_RATE_TARGET=8 \
SESSION_RATE_PEAK=12 \
PRE_ALLOCATED_VUS=64 \
MAX_VUS=256 \
./alt-perf/scripts/run-sv-connect-composite-test.sh
```

高負荷 (2000 ユーザー / 1000 VU) の例:

```bash
USER_COUNT=2000 MAX_VUS=1000 PRE_ALLOCATED_VUS=512 \
SESSION_RATE_BASE=20 SESSION_RATE_TARGET=100 SESSION_RATE_PEAK=120 \
./alt-perf/scripts/run-sv-connect-composite-test.sh
```

## ユーザー数の推定方法

この試験は「同時接続 VU 数」ではなく「セッション流入レート」を測ります。推定は Little の法則で行います。

1. 閾値を満たした最後の plateau を採用する
2. `successful session rate` をその plateau の実効 sessions/sec とみなす
3. 次で同時ユーザー数を推定する

```text
estimated_concurrent_users =
  successful_sessions_per_sec × avg_session_duration_sec
```

安全側に見たいなら `avg` ではなく `p95(session_duration)` を使います。

例:

```text
6.5 sessions/sec × 9.8 sec ≒ 63 users
```

これは「その時点で Alt が同時に抱えていたアクティブユーザー相当」の目安です。限界を探るときは `SESSION_RATE_PEAK` を段階的に上げ、**閾値を最後に通過した run** を採用します。

## 見るべきメトリクス

- `session_success`
- `session_duration`
- `browse_flow_success`
- `read_flow_success`
- `manage_flow_success`
- `rpc_unread_feeds_duration`
- `rpc_all_feeds_duration`
- `rpc_fetch_article_content_duration`
- `rpc_register_rss_duration`
- `http_req_failed`

JSON レポートは `alt-perf/reports/k6-*.json` に出ます。

## pprof プロファイリング

負荷試験時に alt-backend のランタイムプロファイリングを有効化できます。`PPROF_ENABLED=true`（デフォルト有効）を設定すると、alt-backend が `net/http/pprof` エンドポイントをポート 6060 で起動します。

### 自動収集

実行スクリプトは k6 シナリオ終了後、以下のプロファイルを自動収集します:

| プロファイル | 出力先 |
|-------------|--------|
| heap (in-use) | `/tmp/pprof-heap-sv-composite.pb.gz` |
| allocs (cumulative) | `/tmp/pprof-allocs-sv-composite.pb.gz` |
| goroutine | `/tmp/pprof-goroutine-sv-composite.pb.gz` |

### 試験中の手動取得

試験実行中にリアルタイムでプロファイルを取得できます:

```bash
# 30秒間の CPU プロファイル
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# ヒーププロファイル（対話型）
go tool pprof http://localhost:6060/debug/pprof/heap

# goroutine ダンプ（テキスト出力）
curl http://localhost:6060/debug/pprof/goroutine?debug=2

# mutex 競合プロファイル
go tool pprof http://localhost:6060/debug/pprof/mutex

# ブロックプロファイル
go tool pprof http://localhost:6060/debug/pprof/block
```

### 無効化

pprof を無効にする場合:

```bash
PPROF_ENABLED=false ./alt-perf/scripts/run-sv-connect-composite-test.sh
```

## 補足

- 直接 `alt-backend:9101` を叩く既存シナリオより重く出るのは正常です
  - この試験は `alt-frontend-sv` と auth 周辺も含むため
  - 1 RPC あたり nginx → SvelteKit proxy → auth-hub `/validate` → alt-backend の 4 hop を経由する
- 外部依存を real site に向けないため、`FetchArticleContent` も `RegisterRSSFeed` も mock URL のみ使います
- 生成オーバーライド (`compose/sv-connect-load-test-generated.yaml`) はスクリプト終了時に自動削除されます
- 設定オーバーライドの詳細は「インフラ設定のオーバーライド」セクションを参照してください
