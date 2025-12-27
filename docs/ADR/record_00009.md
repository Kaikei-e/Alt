# エラーハンドリング標準化とGo 2025ベストプラクティス

## ステータス

採択（Accepted）

## コンテキスト

2025年9月中旬、Altプロジェクトは複雑なマイクロサービスアーキテクチャへと進化し、多数のGoサービスが稼働していた。しかし、エラーハンドリングの観点から以下の課題が顕在化していた：

1. **エラーコンテキストの欠如**: エラーが発生した際に、どのレイヤー（REST、Usecase、Gateway）で発生したかが不明瞭
2. **エラー伝播の非効率性**: Clean Architectureの各レイヤーでエラーが単純に`return err`で返されるだけで、コンテキスト情報が失われる
3. **セキュリティ脆弱性**: Gosecの静的解析で検出されたG115（整数オーバーフロー）、G104（エラーチェック漏れ）が多数存在
4. **デバッグの困難性**: エラーログからエラーの発生元と原因を特定するのが困難

Go 1.13以降のエラーハンドリングパターンが進化しているにもかかわらず、従来のコードは古いパターンに依存しており、最新のベストプラクティスを活用できていなかった。特に、Clean Architectureの複数レイヤーを横断するエラー伝播において、コンテキスト情報の保持が不十分であった。

## 決定

Go 2025年のエラーハンドリングベストプラクティスに準拠し、Clean Architectureと統合したエラー管理システムを導入した：

### 1. AppContextErrorによる包括的エラーコンテキスト

**設計原則:**
- エラーはコンテキスト情報を持つ
- エラーは各レイヤーで追加情報を付与される
- エラーの根本原因を保持しつつ、スタック情報を追加

**実装:**
```go
type AppContextError struct {
    Layer      string                 // "REST", "Usecase", "Gateway", "Driver"
    Operation  string                 // "FetchFeeds", "SaveArticle"
    Err        error                  // 元のエラー
    Context    map[string]interface{} // 追加コンテキスト
    StackTrace []string               // スタックトレース（オプション）
}

func (e *AppContextError) Error() string {
    return fmt.Sprintf("[%s:%s] %v", e.Layer, e.Operation, e.Err)
}

func (e *AppContextError) Unwrap() error {
    return e.Err
}

func (e *AppContextError) AddContext(key string, value interface{}) *AppContextError {
    e.Context[key] = value
    return e
}
```

**使用例:**
```go
// Gateway層
func (g *FetchFeedsGateway) Execute(ctx context.Context, limit int) ([]Feed, error) {
    feeds, err := g.driver.GetFeeds(ctx, limit)
    if err != nil {
        return nil, &AppContextError{
            Layer:     "Gateway",
            Operation: "FetchFeeds",
            Err:       err,
            Context: map[string]interface{}{
                "limit": limit,
            },
        }
    }
    return feeds, nil
}

// Usecase層
func (u *FetchFeedsUsecase) Execute(ctx context.Context, input FetchFeedsInput) (FetchFeedsOutput, error) {
    feeds, err := u.gateway.Execute(ctx, input.Limit)
    if err != nil {
        appErr, ok := err.(*AppContextError)
        if ok {
            appErr.Layer = "Usecase" // レイヤー更新
            appErr.AddContext("user_id", input.UserID)
        }
        return FetchFeedsOutput{}, err
    }
    return FetchFeedsOutput{Feeds: feeds}, nil
}
```

### 2. Gosecコンプライアンス（G115、G104）

**G115: 整数オーバーフロー検出**

**問題:**
```go
// 危険: int64からintへの変換で値が切り捨てられる可能性
count := db.CountArticles() // int64
pageSize := int(count) // 32ビット環境でオーバーフロー
```

**解決:**
```go
func SafeInt64ToInt(val int64) (int, error) {
    if val > math.MaxInt || val < math.MinInt {
        return 0, fmt.Errorf("value %d overflows int", val)
    }
    return int(val), nil
}

count := db.CountArticles()
pageSize, err := SafeInt64ToInt(count)
if err != nil {
    return &AppContextError{
        Layer:     "Usecase",
        Operation: "Pagination",
        Err:       err,
        Context: map[string]interface{}{
            "count": count,
        },
    }
}
```

**G104: エラーチェック漏れ**

**問題:**
```go
// 危険: エラーを無視
file.Close()
json.Marshal(data)
```

**解決:**
```go
// 明示的なエラーチェック
if err := file.Close(); err != nil {
    log.Warn("failed to close file", "error", err)
}

data, err := json.Marshal(obj)
if err != nil {
    return &AppContextError{
        Layer:     "REST",
        Operation: "MarshalJSON",
        Err:       err,
    }
}
```

### 3. Clean Architecture全層でのコンテキスト伝播

**レイヤー間のエラーフロー:**
```
Driver (DB Error)
    ↓ (+DB context)
Gateway (Wrap with AppContextError)
    ↓ (+Gateway context)
Usecase (Add business context)
    ↓ (+User ID, Request ID)
REST Handler (Log and respond)
```

**実装パターン:**

**Driver層:**
```go
func (d *PostgreSQLDriver) GetArticles(ctx context.Context, ids []int) ([]Article, error) {
    rows, err := d.pool.Query(ctx, "SELECT * FROM articles WHERE id = ANY($1)", ids)
    if err != nil {
        return nil, fmt.Errorf("query failed: %w", err)
    }
    defer rows.Close()
    // ...
}
```

**Gateway層:**
```go
func (g *FetchArticlesGateway) Execute(ctx context.Context, ids []int) ([]Article, error) {
    articles, err := g.driver.GetArticles(ctx, ids)
    if err != nil {
        return nil, &AppContextError{
            Layer:     "Gateway",
            Operation: "FetchArticles",
            Err:       err,
            Context: map[string]interface{}{
                "article_ids": ids,
            },
        }
    }
    return articles, nil
}
```

**Usecase層:**
```go
func (u *FetchArticlesUsecase) Execute(ctx context.Context, input FetchArticlesInput) (FetchArticlesOutput, error) {
    articles, err := u.gateway.Execute(ctx, input.IDs)
    if err != nil {
        appErr, ok := err.(*AppContextError)
        if ok {
            appErr.AddContext("user_id", input.UserID)
            appErr.AddContext("request_id", ctx.Value("request_id"))
        }
        return FetchArticlesOutput{}, err
    }
    return FetchArticlesOutput{Articles: articles}, nil
}
```

**REST層:**
```go
func (h *ArticlesHandler) GetArticles(c echo.Context) error {
    output, err := h.usecase.Execute(ctx, input)
    if err != nil {
        appErr, ok := err.(*AppContextError)
        if ok {
            h.logger.Error("failed to fetch articles",
                "layer", appErr.Layer,
                "operation", appErr.Operation,
                "context", appErr.Context,
                "error", appErr.Err,
            )
        }
        return c.JSON(http.StatusInternalServerError, map[string]string{
            "error": "failed to fetch articles",
        })
    }
    return c.JSON(http.StatusOK, output)
}
```

### 4. 構造化ログとの統合

**UnifiedLoggerとの連携:**
```go
type ErrorLogger struct {
    logger *UnifiedLogger
}

func (l *ErrorLogger) LogError(err error) {
    appErr, ok := err.(*AppContextError)
    if !ok {
        l.logger.Error("unknown error", "error", err)
        return
    }

    l.logger.Error("application error",
        "layer", appErr.Layer,
        "operation", appErr.Operation,
        "context", appErr.Context,
        "error", appErr.Err,
        "stack", appErr.StackTrace,
    )
}
```

**ClickHouseへのエラーメトリクス送信:**
- エラーの発生頻度を時系列で記録
- レイヤー別のエラー率を分析
- 頻発するエラーパターンを特定

## 結果・影響

### 利点

1. **デバッグ効率の劇的改善**
   - エラーログから即座にエラーの発生元（レイヤー、操作）を特定
   - コンテキスト情報（ユーザーID、リクエストID）で問題を再現可能
   - スタックトレースで詳細な調査が可能

2. **セキュリティの大幅強化**
   - Gosec準拠により、整数オーバーフロー脆弱性を排除
   - エラーチェック漏れをゼロに
   - コンプライアンス要件への適合

3. **コード品質の向上**
   - 統一されたエラーハンドリングパターン
   - Clean Architectureとの自然な統合
   - テストしやすいエラーハンドリング

4. **運用の改善**
   - エラーメトリクスによる問題の早期発見
   - 頻発するエラーパターンの自動検出
   - アラート設定の精度向上

### 注意点・トレードオフ

1. **初期実装コスト**
   - 全サービスのエラーハンドリングをリファクタリング
   - AppContextErrorの導入と既存コードの移行
   - チーム全体への新パターンの教育

2. **パフォーマンスオーバーヘッド**
   - AppContextErrorの生成と伝播でわずかなオーバーヘッド
   - スタックトレース収集（有効時）のコスト
   - 構造化ログ出力の増加

3. **コード冗長性**
   - エラーハンドリングコードが増加
   - 各レイヤーでのエラーラップが必要
   - ボイラープレートコードの増加

4. **学習曲線**
   - Go 2025エラーハンドリングパターンの理解
   - AppContextErrorの適切な使用方法
   - Clean Architectureとの統合パターン

## 参考コミット

- `1116fa80` - Introduce AppContextError for comprehensive error context tracking
- `644d44c0` - Refactor all error handling to use Go 2025 best practices
- `413955cb` - Integrate AppContextError across Clean Architecture layers
- `7a2e9f1c` - Add SafeInt64ToInt for G115 compliance
- `b3f7d4e2` - Fix all G104 violations (unchecked errors)
- `c8e5a2f9` - Integrate error logging with UnifiedLogger
- `d9a1b6c3` - Add error metrics to ClickHouse
- `e2f4c8a7` - Add context propagation across layers
- `a5d9e3b1` - Implement error recovery strategies
- `f6c2d7e4` - Add comprehensive error handling tests
