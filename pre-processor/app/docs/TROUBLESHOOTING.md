# Logging Troubleshooting Guide

## 概要

Pre-processorのロギングシステムで発生する可能性のある一般的な問題と解決方法をまとめたガイドです。

---

## 🚨 よくある問題

### 1. fieldsカラムが空で挿入される

#### 症状
- ClickHouseの `fields` カラムが `{}` （空のマップ）になる
- rask-log-aggregatorにログは届くが、カスタムフィールドが抽出されない

#### 原因
1. **JSON構造の不一致**: rask-log-forwarderが期待するslog形式ではない
2. **カスタムフィールドなし**: 標準フィールド（time, level, msg, service）のみでログ出力
3. **フィールド名の競合**: 標準フィールド名と同じ名前を使用

#### 解決方法

```go
// ❌ 悪い例：標準フィールドのみ
logger.Info("operation completed")

// ✅ 良い例：カスタムフィールド追加
logger.Info("operation completed", 
    "duration_ms", 150,
    "status", "success",
    "items_processed", 42)
```

#### 確認方法
```sql
-- ClickHouseでfields確認
SELECT service_name, message, fields, mapLength(fields) as field_count
FROM logs 
WHERE service_name = 'pre-processor' 
  AND timestamp > now() - INTERVAL 1 HOUR
ORDER BY timestamp DESC 
LIMIT 10;
```

---

### 2. ログが出力されない

#### 症状
- アプリケーションが起動するがログが表示されない
- Docker logsで何も表示されない

#### 原因
1. **ログレベル設定**: DEBUGログがINFOレベルで出力されない
2. **出力先設定**: 標準出力以外に出力されている
3. **ロガー初期化エラー**: ロガーが正しく初期化されていない

#### 解決方法

```go
// 1. ログレベル確認
config := logger.LoadLoggerConfigFromEnv()
fmt.Printf("Log level: %s\n", config.Level) // デバッグ用出力

// 2. 強制的にINFOレベルで出力テスト
logger.Logger.Info("Logger test - this should appear")

// 3. 出力先確認
unifiedLogger := logger.NewUnifiedLogger(os.Stdout, "pre-processor") // 確実に標準出力
```

#### 環境変数確認
```bash
# Docker環境での確認
docker exec -it pre-processor env | grep LOG

# 期待される出力
LOG_LEVEL=info
SERVICE_NAME=pre-processor
```

---

### 3. コンテキスト情報が欠損する

#### 症状
- `request_id`, `trace_id`, `operation` がログに含まれない
- 分散トレーシングが機能しない

#### 原因
1. **コンテキスト設定漏れ**: WithRequestID()等の呼び出し忘れ
2. **WithContext()未使用**: コンテキスト対応ロガーを使用していない
3. **コンテキスト伝播エラー**: 関数間でコンテキストが正しく渡されていない

#### 解決方法

```go
// 1. ミドルウェアでコンテキスト設定確認
func LoggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        requestID := r.Header.Get("X-Request-ID")
        if requestID == "" {
            requestID = generateRequestID()
        }
        
        ctx := logger.WithRequestID(r.Context(), requestID)
        ctx = logger.WithOperation(ctx, "http_request")
        
        // ✅ 確実にコンテキスト設定されたリクエスト
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// 2. サービス層でコンテキスト使用確認
func (s *Service) ProcessFeed(ctx context.Context, feedURL string) error {
    // ✅ WithContext()を使用
    contextLogger := s.logger.WithContext(ctx)
    contextLogger.Info("processing started", "feed_url", feedURL)
    
    // ❌ 直接ロガー使用（コンテキスト情報なし）
    // s.logger.Info("processing started", "feed_url", feedURL)
}
```

#### デバッグ方法
```go
// コンテキスト値の確認
func debugContext(ctx context.Context) {
    if requestID := ctx.Value(logger.RequestIDKey); requestID != nil {
        fmt.Printf("RequestID: %s\n", requestID)
    } else {
        fmt.Println("RequestID not found in context")
    }
    
    if traceID := ctx.Value(logger.TraceIDKey); traceID != nil {
        fmt.Printf("TraceID: %s\n", traceID)
    } else {
        fmt.Println("TraceID not found in context")
    }
}
```

---

### 4. パフォーマンス低下

#### 症状
- ログ出力が遅い
- アプリケーションの応答時間が悪化
- メモリ使用量が増加

#### 原因
1. **過度なログ出力**: DEBUGレベルで大量のログ
2. **不適切なレベル設定**: 本番環境でDEBUGレベル
3. **大きなオブジェクトのログ出力**: 巨大なstructやsliceをログ出力

#### 解決方法

```go
// 1. ログレベル最適化
// 本番環境
LOG_LEVEL=info  // DEBUGは避ける

// 開発環境  
LOG_LEVEL=debug // 開発時のみ

// 2. 条件付きログ出力
if logger.Logger.Enabled(context.Background(), slog.LevelDebug) {
    // 重い処理はDEBUGレベルが有効な場合のみ
    expensiveData := generateExpensiveDebugData()
    logger.Logger.Debug("debug info", "data", expensiveData)
}

// 3. 大きなオブジェクトは要約
// ❌ 悪い例
logger.Info("received request", "full_request", largeRequestObject)

// ✅ 良い例  
logger.Info("received request", 
    "method", req.Method,
    "path", req.URL.Path,
    "content_length", req.ContentLength)
```

#### パフォーマンス測定
```go
// ベンチマークテスト
func BenchmarkLogging(b *testing.B) {
    logger := logger.NewUnifiedLogger(io.Discard, "test")
    
    b.ResetTimer()
    b.ReportAllocs()
    
    for i := 0; i < b.N; i++ {
        logger.Info("benchmark test", "iteration", i)
    }
}
```

---

### 5. JSON形式エラー

#### 症状
- ログがJSON形式でない
- rask-log-forwarderがログを解析できない
- ClickHouseにデータが挿入されない

#### 原因
1. **text形式設定**: JSON以外の形式で出力
2. **カスタムハンドラー**: slog.NewJSONHandler以外使用
3. **改行文字問題**: 複数行ログで JSON が壊れる

#### 解決方法

```go
// 1. JSON形式確認
logger := logger.NewUnifiedLogger(os.Stdout, "pre-processor")

// 出力例確認
logger.Info("test message", "key", "value")
// 期待される出力: {"time":"2024-01-01T12:00:00Z","level":"INFO","msg":"test message","key":"value","service":"pre-processor"}

// 2. 改行文字を含むメッセージの処理
message := "line1\nline2\nline3"
// ❌ そのまま出力するとJSON が壊れる
logger.Info(message)

// ✅ エスケープまたは要約
logger.Info("multiline message", "line_count", 3, "preview", strings.Replace(message, "\n", "\\n", -1))
```

#### JSON検証
```bash
# ログ出力のJSON検証
docker logs pre-processor 2>&1 | tail -n 1 | jq .

# 成功例
{
  "time": "2024-01-01T12:00:00Z",
  "level": "INFO", 
  "msg": "test message",
  "service": "pre-processor"
}

# エラー例（parse error if invalid JSON）
parse error: Invalid numeric literal at line 1, column 10
```

---

## 🔧 診断コマンド

### ログ出力確認
```bash
# 1. 最新のログ確認
docker logs pre-processor --tail 10

# 2. リアルタイムログ監視
docker logs pre-processor -f

# 3. 特定時間範囲のログ
docker logs pre-processor --since "2024-01-01T10:00:00" --until "2024-01-01T11:00:00"
```

### ClickHouse確認
```sql
-- 1. 最新ログ確認
SELECT * FROM logs 
WHERE service_name = 'pre-processor' 
ORDER BY timestamp DESC 
LIMIT 10;

-- 2. fieldsカラム確認
SELECT service_name, message, fields, mapLength(fields) as field_count
FROM logs 
WHERE service_name = 'pre-processor' 
  AND timestamp > now() - INTERVAL 1 HOUR
  AND mapLength(fields) > 0;

-- 3. ログレベル分布
SELECT level, count() as count
FROM logs 
WHERE service_name = 'pre-processor' 
  AND timestamp > now() - INTERVAL 1 HOUR
GROUP BY level;
```

### コンテナヘルスチェック
```bash
# 1. コンテナ状態確認
docker ps | grep pre-processor

# 2. rask-log-forwarder確認
docker ps | grep rask-log-forwarder

# 3. ネットワーク確認
docker network ls
docker network inspect alt-network
```

---

## 🚑 緊急対応手順

### 1. ログが全く出力されない場合

```bash
# Step 1: コンテナ状態確認
docker ps -a | grep pre-processor

# Step 2: 強制再起動
docker restart pre-processor

# Step 3: ログレベルを一時的にDEBUGに変更
docker exec -it pre-processor sh -c 'export LOG_LEVEL=debug'

# Step 4: 手動ログテスト
docker exec -it pre-processor sh -c 'echo "Manual test log" >> /proc/1/fd/1'
```

### 2. rask-log-aggregator連携が停止した場合

```bash
# Step 1: rask-log-forwarder確認
docker logs rask-log-forwarder --tail 20

# Step 2: rask-log-aggregator確認  
docker logs rask-log-aggregator --tail 20

# Step 3: 段階的再起動
docker restart rask-log-forwarder
sleep 10
docker restart rask-log-aggregator

# Step 4: 接続確認
curl -f http://rask-log-aggregator:9600/health || echo "rask-log-aggregator not responding"
```

### 3. パフォーマンス問題の緊急対応

```bash
# Step 1: ログレベルを一時的にERRORに変更
docker exec -it pre-processor sh -c 'export LOG_LEVEL=error'

# Step 2: メモリ使用量確認
docker stats pre-processor --no-stream

# Step 3: 必要に応じてコンテナ再起動
if [[ $(docker stats pre-processor --no-stream --format "{{.MemPerc}}" | sed 's/%//') -gt 80 ]]; then
    echo "High memory usage detected, restarting container"
    docker restart pre-processor
fi
```

---

## 🧪 テスト手順

### ログ出力テスト

```go
// 1. 基本ログ出力テスト
func TestBasicLogging() {
    logger := logger.NewUnifiedLogger(os.Stdout, "test-service")
    logger.Info("test message", "test_key", "test_value")
    // 期待: JSON形式で出力される
}

// 2. コンテキストログテスト
func TestContextLogging() {
    ctx := logger.WithRequestID(context.Background(), "test-req-123")
    logger := logger.NewUnifiedLogger(os.Stdout, "test-service")
    contextLogger := logger.WithContext(ctx)
    contextLogger.Info("context test")
    // 期待: request_idがログに含まれる
}
```

### 統合テスト

```bash
#!/bin/bash
# integration_test.sh

echo "=== Pre-processor Logging Integration Test ==="

# 1. ログ出力テスト
echo "1. Testing log output..."
docker exec pre-processor sh -c 'echo "Integration test log" | logger'

# 2. JSON形式確認
echo "2. Checking JSON format..."
LOG_LINE=$(docker logs pre-processor --tail 1)
echo "$LOG_LINE" | jq . > /dev/null && echo "✅ Valid JSON" || echo "❌ Invalid JSON"

# 3. rask-log-aggregator連携確認
echo "3. Checking rask-log-aggregator integration..."
sleep 5
RECENT_LOGS=$(docker exec clickhouse clickhouse-client --query "SELECT count() FROM logs WHERE service_name = 'pre-processor' AND timestamp > now() - INTERVAL 1 MINUTE")
if [[ $RECENT_LOGS -gt 0 ]]; then
    echo "✅ Logs reaching ClickHouse: $RECENT_LOGS"
else
    echo "❌ No recent logs in ClickHouse"
fi

echo "=== Test Complete ==="
```

---

## 📞 サポート情報

### ログ収集
問題報告時は以下の情報を含めてください：

```bash
# 1. 環境情報
docker --version
docker-compose --version
echo "SERVICE_NAME: $SERVICE_NAME"
echo "LOG_LEVEL: $LOG_LEVEL"

# 2. コンテナ状態
docker ps -a | grep -E "(pre-processor|rask-log)"

# 3. 最新ログ
docker logs pre-processor --tail 50
docker logs rask-log-forwarder --tail 20
docker logs rask-log-aggregator --tail 20

# 4. ClickHouse状態
docker exec clickhouse clickhouse-client --query "SELECT count() FROM logs WHERE timestamp > now() - INTERVAL 1 HOUR"
```

### 設定確認チェックリスト

- [ ] `LOG_LEVEL` 環境変数が設定されている
- [ ] `SERVICE_NAME` 環境変数が設定されている  
- [ ] Docker logsでJSON形式のログが出力されている
- [ ] rask-log-forwarderコンテナが起動している
- [ ] rask-log-aggregatorコンテナが起動している
- [ ] ClickHouseでlogsテーブルにデータが挿入されている
- [ ] fieldsカラムにカスタムフィールドが含まれている

### パフォーマンス監視

```bash
# 定期監視スクリプト
#!/bin/bash
while true; do
    MEMORY=$(docker stats pre-processor --no-stream --format "{{.MemPerc}}" | sed 's/%//')
    CPU=$(docker stats pre-processor --no-stream --format "{{.CPUPerc}}" | sed 's/%//')
    
    echo "$(date): Memory: ${MEMORY}%, CPU: ${CPU}%"
    
    if (( $(echo "$MEMORY > 80" | bc -l) )); then
        echo "WARNING: High memory usage detected"
    fi
    
    sleep 60
done
```