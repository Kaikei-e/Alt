# CLAUDE.md - Pre-Processor-Sidecar Service

<!-- Model Configuration -->
<!-- ALWAYS use claude-4-sonnet for this project -->

## 🎯 Service Overview

**Pre-Processor-Sidecar** は、Inoreader APIと連携してRSSフィード情報を30分間隔で同期するKubernetes CronJobです。

### Core Mission
- **30分間隔実行**: API制限（100 requests/日）内で安全運用
- **Envoy Proxy経由**: セキュアな外部通信（RBAC制御）
- **OAuth2管理**: リフレッシュトークン自動更新
- **個人利用最適化**: 監視機能なし、軽量リソース

---

## 🏗️ Clean Architecture (簡素化版)

```
/pre-processor-sidecar/app/
├─ handler/     # CronJob実行エントリーポイント
├─ service/     # OAuth2, API連携, データ処理 (PRIMARY TEST TARGET)
├─ repository/  # データベース操作
├─ driver/      # 外部連携 (Envoy HTTP, OAuth2, DB)
├─ models/      # ドメインエンティティ
└─ config/      # 設定管理
```

**Layer Dependencies**: Handler → Service → Repository → Driver

---

## 🔴🟢🔵 TDD Rules (NON-NEGOTIABLE)

### 1. RED-GREEN-REFACTOR Cycle
```go
// 1. RED: 失敗するテストを最初に書く
func TestOAuth2Service_RefreshToken(t *testing.T) {
    tests := map[string]struct {
        refreshToken string
        expectError  bool
    }{
        "valid_token": {
            refreshToken: "valid_refresh_token",
            expectError: false,
        },
    }

    for name, tc := range tests {
        t.Run(name, func(t *testing.T) {
            service := NewOAuth2Service(mockClient)
            _, err := service.RefreshToken(tc.refreshToken)
            
            if tc.expectError {
                require.Error(t, err)
            } else {
                require.NoError(t, err)
            }
        })
    }
}

// 2. GREEN: 最小限の実装でテスト通過
// 3. REFACTOR: エラーハンドリング・ログ追加
```

---

## 📊 Inoreader API Specifications

### API制限 (CRITICAL)
- **Zone 1 (読み取り)**: 100 requests/日
- **30分間隔実行**: 48 calls/日 < 100制限 ✅
- **レスポンスヘッダー監視**: `X-Reader-Zone1-Usage`, `X-Reader-Zone1-Limit`

### 主要エンドポイント
```go
const (
    SubscriptionListAPI = "/subscription/list"     // Zone 1
    StreamContentsAPI   = "/stream/contents/{id}"  // Zone 1  
    OAuth2TokenAPI      = "/oauth2/token"          // Token refresh
)
```

### OAuth2フロー
```go
type OAuth2Config struct {
    ClientID     string
    ClientSecret string
    RefreshToken string
    AccessToken  string
    ExpiresAt    time.Time
}

// トークンリフレッシュ (5分前に実行)
func (c *OAuth2Config) NeedsRefresh() bool {
    return time.Until(c.ExpiresAt) < 5*time.Minute
}
```

---

## 🌐 Envoy Proxy Integration

### HTTP Client Configuration
```go
func NewEnvoyHTTPClient() *http.Client {
    return &http.Client{
        Transport: &http.Transport{
            Proxy: http.ProxyFromEnvironment, // HTTPS_PROXY env var
            DialContext: (&net.Dialer{
                Timeout: 10 * time.Second,
            }).DialContext,
            TLSHandshakeTimeout: 10 * time.Second,
        },
        Timeout: 30 * time.Second,
    }
}
```

### Environment Variables
```go
// Required proxy configuration
HTTPS_PROXY=http://envoy-proxy.alt-apps.svc.cluster.local:8081
NO_PROXY=localhost,127.0.0.1,.svc.cluster.local
```

---

## 🗃️ Database Schema (Extended)

### Inoreader Integration Tables
```sql
-- 購読フィード管理
CREATE TABLE inoreader_subscriptions (
    id UUID PRIMARY KEY,
    inoreader_id TEXT UNIQUE NOT NULL,
    feed_url TEXT NOT NULL,
    title TEXT,
    category TEXT,
    synced_at TIMESTAMP DEFAULT NOW()
);

-- 記事メタデータ
CREATE TABLE inoreader_articles (
    id UUID PRIMARY KEY,
    inoreader_id TEXT UNIQUE NOT NULL,
    subscription_id UUID REFERENCES inoreader_subscriptions(id),
    article_url TEXT NOT NULL,
    title TEXT,
    published_at TIMESTAMP,
    fetched_at TIMESTAMP DEFAULT NOW()
);

-- API使用量追跡 (30分間隔対応)
CREATE TABLE api_usage_tracking (
    id UUID PRIMARY KEY,
    date DATE DEFAULT CURRENT_DATE,
    zone1_requests INT DEFAULT 0,
    last_reset TIMESTAMP DEFAULT NOW(),
    rate_limit_headers JSONB
);

-- 継続トークン管理
CREATE TABLE sync_state (
    id UUID PRIMARY KEY,
    stream_id TEXT NOT NULL,
    continuation_token TEXT,
    last_sync TIMESTAMP DEFAULT NOW()
);
```

---

## 🧪 Testing Strategy

### Service Layer Tests (90%+ Coverage)
```go
//go:generate mockgen -source=oauth2_client.go -destination=../mocks/mock_oauth2_client.go

func TestInoreaderService_SyncSubscriptions(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockOAuth2 := mocks.NewMockOAuth2Client(ctrl)
    mockHTTP := mocks.NewMockHTTPClient(ctrl)
    
    mockOAuth2.EXPECT().
        EnsureValidToken(gomock.Any()).
        Return(nil)
    
    mockHTTP.EXPECT().
        Get("https://www.inoreader.com/reader/api/0/subscription/list").
        Return(&http.Response{
            StatusCode: 200,
            Body: io.NopCloser(strings.NewReader(`{"subscriptions":[]}`)),
        }, nil)

    service := NewInoreaderService(mockOAuth2, mockHTTP)
    err := service.SyncSubscriptions(context.Background())
    
    require.NoError(t, err)
}
```

### Integration Tests (Rate Limited)
```go
func TestInoreaderIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }
    
    // Real API calls with rate limiting
    client := NewRateLimitedClient(30 * time.Minute)
    service := NewInoreaderService(client)
    
    // Test with real Envoy proxy
    subscriptions, err := service.FetchSubscriptions(context.Background())
    require.NoError(t, err)
    assert.NotEmpty(t, subscriptions)
}
```

---

## 📝 Coding Standards

### File Headers
```go
// ABOUTME: This file handles Inoreader OAuth2 token management and refresh
// ABOUTME: Ensures tokens are valid before API calls with 5-minute buffer
```

### Structured Logging
```go
func (s *InoreaderService) SyncSubscriptions(ctx context.Context) error {
    logger := s.logger.With("operation", "sync_subscriptions")
    
    start := time.Now()
    logger.Info("starting subscription sync")
    
    // API制限チェック
    if s.rateLimiter.ExceedsLimit() {
        logger.Error("API daily limit exceeded", 
            "limit", 100,
            "current_usage", s.rateLimiter.GetUsage())
        return ErrAPILimitExceeded
    }
    
    // API呼び出し...
    
    logger.Info("subscription sync completed",
        "duration_ms", time.Since(start).Milliseconds(),
        "subscriptions_count", len(subscriptions),
        "api_usage", s.rateLimiter.GetUsageInfo())
    
    return nil
}
```

---

## 🔒 Security Requirements

### Network Security
- **NetworkPolicy**: Envoy Proxy (`port 8081`) のみegress許可
- **Envoy RBAC**: `www.inoreader.com`, `inoreader.com`のみ許可
- **Pod Security**: 非root、読み取り専用ファイルシステム

### Secret Management
```yaml
# values-production.yaml
secret:
  data:
    INOREADER_CLIENT_ID: "your_client_id_here"
    INOREADER_CLIENT_SECRET: "your_client_secret_here"
    INOREADER_REFRESH_TOKEN: "your_refresh_token_here"
```

---

## ⚡ Performance Targets

### Resource Usage
- **Memory**: 128Mi limit (軽量CronJob)
- **CPU**: 100m limit (API呼び出しのみ)
- **実行時間**: 25分以内 (30分間隔)

### API効率
- **バッチサイズ**: 100記事/回 (Inoreader制限)
- **継続トークン**: ページング処理最適化
- **レート制限**: 48回/日 < 100回制限

---

## 🚨 Error Handling

### API制限対応
```go
type APILimitError struct {
    CurrentUsage int
    DailyLimit   int
    ResetTime    time.Time
}

func (e *APILimitError) Error() string {
    return fmt.Sprintf("API limit exceeded: %d/%d requests used", 
        e.CurrentUsage, e.DailyLimit)
}

// 429エラー時は次回CronJob実行まで待機
func (s *InoreaderService) handleRateLimit(resp *http.Response) error {
    if resp.StatusCode == 429 {
        s.logger.Warn("rate limited, will retry next execution")
        return &APILimitError{
            CurrentUsage: s.parseUsageHeader(resp),
            DailyLimit:   100,
            ResetTime:    time.Now().Add(30 * time.Minute),
        }
    }
    return nil
}
```

---

## 📋 Success Criteria

### Development Checklist
- [ ] TDD優先開発 (RED-GREEN-REFACTOR)
- [ ] Service層90%+テストカバレッジ
- [ ] OAuth2トークン自動管理
- [ ] API制限監視機能
- [ ] 継続トークン永続化
- [ ] Envoy Proxy経由通信確認

### Production Readiness
- [ ] 30分間隔CronJob動作確認
- [ ] NetworkPolicy egress制限
- [ ] Secret値の本番設定
- [ ] API使用量ダッシュボード(簡易)
- [ ] エラー回復機能テスト

---

**Remember**: Domain understanding drives implementation. TDD ensures quality. API limits guide architecture. Simplicity enables maintainability.