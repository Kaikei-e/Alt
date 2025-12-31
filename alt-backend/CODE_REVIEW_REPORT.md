# Alt-Backend コードレビューレポート

**レビュー日**: 2025-12-31
**対象**: alt-backend (Go 1.24+, Echo Framework)
**ファイル数**: 410 Go files, 114 test files
**アーキテクチャ**: Clean Architecture (5層パターン)

---

## エグゼクティブサマリー

alt-backendは、全体的に高品質なGoコードベースであり、Clean Architectureの原則に従って適切に構造化されています。しかし、golangci-lintで検出された問題と、いくつかのベストプラクティス違反が確認されました。

### 評価サマリー

| カテゴリ | 評価 | コメント |
|---------|------|----------|
| セキュリティ | ⭐⭐⭐⭐⭐ | 優秀 - SSRF, DoS, 認証が適切に実装 |
| パフォーマンス | ⭐⭐⭐⭐ | 良好 - レート制限、接続プール管理が適切 |
| テスト品質 | ⭐⭐⭐⭐ | 良好 - テーブル駆動テスト、適切なモック使用 |
| エラーハンドリング | ⭐⭐⭐ | 改善の余地あり - 多数の未チェックエラー |
| ドキュメント | ⭐⭐⭐ | 改善の余地あり - docコメント不足 |
| コード品質 | ⭐⭐⭐⭐ | 良好 - 未使用コードの削除が必要 |

---

## 1. 静的解析結果 (golangci-lint)

### 1.1 エラーチェック漏れ (errcheck) - 高優先度

**検出数**: 40件以上

主な問題箇所:

| ファイル | 行 | 問題 |
|----------|-----|------|
| `driver/alt_db/save_article_driver.go` | 88 | `tx.Rollback(ctx)` 未チェック |
| `driver/alt_db/tenant_repository.go` | 38, 184 | `tx.Rollback` 未チェック |
| `job/job_runner.go` | 51 | `r.RegisterMultipleFeeds` 未チェック |
| `rest/augur_handler.go` | 123, 126 | `c.Response().Write` 未チェック |
| `rest/sse_handlers.go` | 109 | `c.Response().Write` 未チェック |
| `utils/html_parser/parser.go` | 49 | `gzr.Close()` 未チェック |

**推奨対応**:
```go
// Before (問題あり)
tx.Rollback(ctx)

// After (推奨)
if err := tx.Rollback(ctx); err != nil {
    slog.Error("rollback failed", "error", err)
}
```

### 1.2 静的解析警告 (staticcheck)

| 問題 | ファイル | 説明 |
|------|----------|------|
| SA9003 | `adapter/augur_adapter/augur_adapter.go:107` | 空のif文ブロック |
| SA9003 | `gateway/scraping_policy_gateway/scraping_policy_gateway.go:71, 103` | 空のif文ブロック |
| S1000 | `driver/csrf_token_driver/csrf_token_driver.go:81` | `for range` を使用すべき |
| S1016 | `gateway/feed_search_gateway/search_feed_meilisearch_gateway.go:50, 107` | 構造体変換を使用すべき |
| SA1029 | `middleware/jwt_middleware.go:94` | context keyに組み込み型を使用 |
| SA4017 | `gateway/robots_txt_gateway/robots_txt_gateway.go:178` | 戻り値が無視されている |

### 1.3 未使用コード (unused)

| ファイル | 対象 |
|----------|------|
| `driver/alt_db/init.go:106` | `func envChecker` |
| `driver/csrf_token_driver/csrf_token_driver.go:15` | `field mu` |
| `gateway/register_feed_gateway/single_feed_link_gateway.go:348, 389` | 未使用メソッド |
| `middleware/auth_middleware.go:30` | `var errInvalidSecret` |
| `connect/v2/articles/handler_test.go:16` | `func createAuthContext` |
| `connect/v2/rss/handler_test.go:16` | `func createAuthContext` |

---

## 2. セキュリティレビュー

### 2.1 優れた実装

#### SSRF対策 (`utils/security/ssrf_validator.go`)
- DNS リバインディング防止
- メタデータエンドポイントブロック (AWS/Azure/GCP)
- プライベートIP検出
- Unicode/Punycode攻撃防止
- TOCTOU攻撃検出
- 接続時IP検証 (Safeurl方式)

```go
// 優れた実装例
func (v *SSRFValidator) validateDNSRebinding(ctx context.Context, u *url.URL) error {
    // TOCTOU検出のための再解決
    if v.isSuspiciousDomain(hostname) {
        time.Sleep(100 * time.Millisecond)
        ips2, err := v.resolveWithTimeout(ctx, hostname, 5*time.Second)
        // ...
    }
}
```

#### DoS対策 (`middleware/dos_protection_middleware.go`)
- IPベースレート制限
- サーキットブレーカーパターン
- 設定可能なバースト制限
- ホワイトリストパス対応
- SSEエンドポイント例外処理

#### 認証 (`middleware/auth_middleware.go`)
- JWT + Shared Secret のデュアル認証
- 適切なエラーハンドリング
- セキュアなフェイルファースト設計

### 2.2 セキュリティ改善提案

| 優先度 | 問題 | 推奨対応 |
|--------|------|----------|
| 中 | Context keyに `string` 型を使用 | カスタム型を定義して衝突を防止 |
| 低 | CSRFミドルウェアが無効化 | 本番環境で有効化を検討 |

```go
// Before (middleware/jwt_middleware.go:94)
ctx := context.WithValue(c.Request().Context(), userContextKey, userCtx)

// After (推奨) - domain/user_context.go で既に実装済み
type contextKey string
const UserContextKey contextKey = "user_context"
```

---

## 3. パフォーマンスレビュー

### 3.1 優れた実装

#### DB接続プール (`driver/alt_db/init.go`)
- 適切なプールサイズ設定
- リトライロジック (指数バックオフ)
- ヘルスチェック
- Linkerd mTLS対応

#### レート制限 (`utils/rate_limiter/rate_limiter.go`)
- ホストベースの制限
- Double-check lockingパターン
- `golang.org/x/time/rate` 使用

### 3.2 パフォーマンス問題

| 優先度 | 問題 | ファイル | 推奨対応 |
|--------|------|----------|----------|
| 高 | goroutineの無限ループ | `job/job_runner.go:26-57` | 明確な終了シグナル追加 |
| 中 | レート制限マップのクリーンアップなし | `rate_limiter.go` | 定期的なクリーンアップ追加 |
| 中 | サーキットブレーカーのロック競合 | `dos_protection_middleware.go:257-269` | RWMutexの再取得を改善 |

```go
// 問題のあるコード (job/job_runner.go)
go func() {
    for {  // 無限ループ - 終了条件なし
        // ...
        time.Sleep(1 * time.Hour)
    }
}()

// 推奨
go func() {
    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // process
        }
    }
}()
```

### 3.3 サーキットブレーカーのロック問題

```go
// 問題のあるコード (dos_protection_middleware.go:257-263)
func (cb *circuitBreaker) shouldBlock() bool {
    cb.mu.RLock()
    defer cb.mu.RUnlock()
    // ...
    if time.Since(cb.lastFailureTime) > cb.config.RecoveryTimeout {
        cb.mu.RUnlock()  // deferでもRUnlockされる - 問題の可能性
        cb.mu.Lock()
        cb.state = circuitHalfOpen
        cb.mu.Unlock()
        cb.mu.RLock()  // deferでRUnlockされる
        return false
    }
}
```

---

## 4. テスト品質レビュー

### 4.1 優れた実装

#### テーブル駆動テスト
`usecase/fetch_feed_usecase/single_feed_usecase_test.go` は優れたテストパターンを示しています:

```go
tests := []struct {
    name      string
    mockSetup func(*mocks.MockFetchSingleFeedPort)
    want      *domain.RSSFeed
    wantErr   bool
    errorType string
}{
    // 9つのテストケース
}
```

#### エッジケースのカバレッジ
- 特殊文字 (Unicode, 絵文字)
- 長いタイトル
- 空のフィード
- 大量アイテム (1000件)
- コンテキストプロパゲーション
- nilロガー対応

### 4.2 テスト品質の問題

| 優先度 | 問題 | 推奨対応 |
|--------|------|----------|
| 中 | 未使用のテスト関数 | `createAuthContext` を削除または使用 |
| 中 | テストでのエラー無視 | `os.Setenv` のエラーをチェック |
| 低 | テストでのハードコードされたコンテキストキー | `test_utils/test_helpers.go:90-91` で型付きキーを使用 |

### 4.3 テストカバレッジ

- **テストファイル数**: 114 files
- **テスト対象レイヤー**: Usecase, Gateway
- **モック使用**: GoMock (uber-go/mock)
- **アサーション**: testify/require, testify/assert

---

## 5. 一般的なベストプラクティス

### 5.1 優れた実装

| カテゴリ | 実装状況 |
|----------|----------|
| Clean Architecture | 5層パターン厳密に遵守 |
| 依存性注入 | DIコンテナで一元管理 |
| 構造化ロギング | `log/slog` 使用 |
| エラー型 | カスタムAppError/AppContextError |
| Context伝播 | 全レイヤーで適切に伝播 |
| レート制限 | 5秒最小間隔を遵守 |

### 5.2 改善が必要な領域

#### パッケージ命名 (ベストプラクティス違反)

```
utils/              # 汎用的すぎる
utils/errors/       # 改善案: apperrors/
utils/security/     # OK - 具体的
utils/rate_limiter/ # OK - 具体的
```

#### Context キー

```go
// 問題 (test_utils/test_helpers.go:90-91)
ctx = context.WithValue(ctx, "test_name", t.Name())

// 推奨
type testContextKey string
const testNameKey testContextKey = "test_name"
ctx = context.WithValue(ctx, testNameKey, t.Name())
```

#### エラーメッセージ

一部のエラーメッセージが大文字で始まっています (Goの慣例では小文字):

```go
// 一部のファイルで見られる
return fmt.Errorf("Database connection failed")  // 大文字

// 推奨
return fmt.Errorf("database connection failed")  // 小文字
```

---

## 6. 推奨事項 (優先度順)

### 高優先度

1. **エラーチェック追加** (40件以上)
   - トランザクションロールバックのエラーチェック
   - `defer` 内のClose呼び出しのエラーログ
   - HTTP応答書き込みのエラーハンドリング

2. **goroutine終了条件追加**
   - `job/job_runner.go` にcontext cancellationを追加
   - バックグラウンドジョブに明確な終了シグナルを実装

3. **サーキットブレーカーのロック修正**
   - `dos_protection_middleware.go` のRWMutex使用パターンを修正

### 中優先度

4. **未使用コード削除**
   - `envChecker`, `errInvalidSecret`, `createAuthContext` など
   - 未使用の構造体フィールド `mu` in csrf_token_driver

5. **Context キー型の統一**
   - すべてのcontextキーに型付きキーを使用

6. **空のif文ブロック修正**
   - `adapter/augur_adapter/augur_adapter.go:107`
   - `gateway/scraping_policy_gateway/scraping_policy_gateway.go`

### 低優先度

7. **構造体変換の簡略化**
   - `gateway/feed_search_gateway/search_feed_meilisearch_gateway.go`

8. **テストでのos.Setenvエラーチェック**
   - `config/config_test.go`

9. **ドキュメントコメント追加**
   - エクスポートされた型と関数にdocコメントを追加

---

## 7. 参考資料

- [Effective Go](https://golang.org/doc/effective_go.html)
- [Microsoft Go Code Review](https://microsoft.github.io/code-with-engineering-playbook/code-reviews/recipes/go/)
- [Practical Go by Dave Cheney](https://dave.cheney.net/practical-go/presentations/qcon-china.html)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

---

## 付録: golangci-lint 設定推奨

現在のプロジェクトには `.golangci.yml` が見つかりませんでした。以下の設定を推奨します:

```yaml
# .golangci.yml
linters:
  enable:
    - errcheck
    - staticcheck
    - unused
    - govet
    - gosec
    - gofmt
    - goimports

linters-settings:
  errcheck:
    check-blank: true
    check-type-assertions: true

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - errcheck
```
