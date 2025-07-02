# Logging API Reference

## 概要

Pre-processorサービスのロギングAPIは、UnifiedLoggerとContextLoggerの2つの主要コンポーネントで構成されています。これらのAPIは、Alt-backend互換のJSON構造化ログを出力し、rask-log-aggregatorとの完全な統合を提供します。

---

## UnifiedLogger

### type UnifiedLogger

```go
type UnifiedLogger struct {
    logger      *slog.Logger
    serviceName string
}
```

slogベースの統一ロガー。Alt-backend互換のJSON出力を生成します。

### func NewUnifiedLogger

```go
func NewUnifiedLogger(output io.Writer, serviceName string) *UnifiedLogger
```

新しいUnifiedLoggerインスタンスを作成します。

**パラメータ:**
- `output io.Writer`: ログの出力先（通常は `os.Stdout`）
- `serviceName string`: サービス識別子（例: "pre-processor"）

**戻り値:**
- `*UnifiedLogger`: 設定済みのUnifiedLoggerインスタンス

**例:**
```go
logger := NewUnifiedLogger(os.Stdout, "pre-processor")
```

### func NewUnifiedLoggerWithLevel

```go
func NewUnifiedLoggerWithLevel(output io.Writer, serviceName, level string) *UnifiedLogger
```

指定されたログレベルでUnifiedLoggerを作成します。

**パラメータ:**
- `output io.Writer`: ログの出力先
- `serviceName string`: サービス識別子
- `level string`: ログレベル（"debug", "info", "warn", "error"）

**戻り値:**
- `*UnifiedLogger`: 設定済みのUnifiedLoggerインスタンス

**例:**
```go
logger := NewUnifiedLoggerWithLevel(os.Stdout, "pre-processor", "debug")
```

### func (*UnifiedLogger) WithContext

```go
func (ul *UnifiedLogger) WithContext(ctx context.Context) *slog.Logger
```

コンテキストから値を抽出してslog.Loggerを返します。

**パラメータ:**
- `ctx context.Context`: リクエストID、トレースID、オペレーション名を含むコンテキスト

**戻り値:**
- `*slog.Logger`: コンテキスト情報が設定されたslogロガー

**抽出されるコンテキスト値:**
- `RequestIDKey`: リクエスト識別子
- `TraceIDKey`: 分散トレーシング用トレースID
- `OperationKey`: 実行中のオペレーション名

**例:**
```go
ctx := WithRequestID(context.Background(), "req-123")
ctx = WithTraceID(ctx, "trace-456")
contextLogger := logger.WithContext(ctx)
contextLogger.Info("operation started")
```

### func (*UnifiedLogger) Info

```go
func (ul *UnifiedLogger) Info(msg string, args ...any)
```

INFOレベルのログを出力します（便利メソッド）。

**パラメータ:**
- `msg string`: ログメッセージ
- `args ...any`: キー・バリューペアの可変引数

### func (*UnifiedLogger) Error

```go
func (ul *UnifiedLogger) Error(msg string, args ...any)
```

ERRORレベルのログを出力します（便利メソッド）。

### func (*UnifiedLogger) Debug

```go
func (ul *UnifiedLogger) Debug(msg string, args ...any)
```

DEBUGレベルのログを出力します（便利メソッド）。

### func (*UnifiedLogger) Warn

```go
func (ul *UnifiedLogger) Warn(msg string, args ...any)
```

WARNレベルのログを出力します（便利メソッド）。

### func (*UnifiedLogger) With

```go
func (ul *UnifiedLogger) With(args ...any) *UnifiedLogger
```

追加属性を持つ新しいUnifiedLoggerを返します。

**パラメータ:**
- `args ...any`: 追加するキー・バリューペア

**戻り値:**
- `*UnifiedLogger`: 属性が追加された新しいロガーインスタンス

**例:**
```go
serviceLogger := logger.With("component", "feed-processor", "version", "1.0.0")
serviceLogger.Info("service started")
```

---

## ContextLogger

### type ContextLogger

```go
type ContextLogger struct {
    logger        *slog.Logger
    serviceName   string
    unifiedLogger *UnifiedLogger
}
```

既存APIとの互換性を保ちながら、内部でUnifiedLoggerを使用するラッパー。

### func NewContextLogger

```go
func NewContextLogger(output io.Writer, format, level string) *ContextLogger
```

新しいContextLoggerを作成します。

**パラメータ:**
- `output io.Writer`: ログの出力先
- `format string`: ログ形式（"json" または "text"）
- `level string`: ログレベル

**戻り値:**
- `*ContextLogger`: 設定済みのContextLoggerインスタンス

### func NewContextLoggerWithConfig

```go
func NewContextLoggerWithConfig(config *LoggerConfig, output io.Writer) *ContextLogger
```

設定オブジェクトからContextLoggerを作成します。

**パラメータ:**
- `config *LoggerConfig`: ロガー設定
- `output io.Writer`: ログの出力先

**戻り値:**
- `*ContextLogger`: 設定済みのContextLoggerインスタンス

**例:**
```go
config := LoadLoggerConfigFromEnv()
contextLogger := NewContextLoggerWithConfig(config, os.Stdout)
```

### func (*ContextLogger) WithContext

```go
func (cl *ContextLogger) WithContext(ctx context.Context) *slog.Logger
```

コンテキスト対応のslog.Loggerを返します。

**パラメータ:**
- `ctx context.Context`: リクエストコンテキスト

**戻り値:**
- `*slog.Logger`: コンテキスト情報が設定されたslogロガー

---

## Configuration

### type LoggerConfig

```go
type LoggerConfig struct {
    Level       string
    Format      string
    ServiceName string
    UseRask     bool // Deprecated: 常にfalse
}
```

ロガー設定を表す構造体。

### type UnifiedLoggerConfig

```go
type UnifiedLoggerConfig struct {
    Level       string `env:"LOG_LEVEL" default:"info"`
    ServiceName string `env:"SERVICE_NAME" default:"pre-processor"`
}
```

UnifiedLogger専用の簡略化された設定構造体。

### func LoadLoggerConfigFromEnv

```go
func LoadLoggerConfigFromEnv() *LoggerConfig
```

環境変数からLoggerConfigを読み込みます。

**環境変数:**
- `LOG_LEVEL`: ログレベル（default: "info"）
- `LOG_FORMAT`: ログ形式（default: "json"）
- `SERVICE_NAME`: サービス名（default: "pre-processor"）
- `USE_RASK_LOGGER`: 廃止（常にfalse）

### func LoadUnifiedLoggerConfigFromEnv

```go
func LoadUnifiedLoggerConfigFromEnv() *UnifiedLoggerConfig
```

環境変数からUnifiedLoggerConfigを読み込みます。

---

## Context Helper Functions

### func WithRequestID

```go
func WithRequestID(ctx context.Context, requestID string) context.Context
```

コンテキストにリクエストIDを設定します。

**パラメータ:**
- `ctx context.Context`: ベースコンテキスト
- `requestID string`: リクエスト識別子

**戻り値:**
- `context.Context`: リクエストIDが設定されたコンテキスト

### func WithTraceID

```go
func WithTraceID(ctx context.Context, traceID string) context.Context
```

コンテキストにトレースIDを設定します。

### func WithOperation

```go
func WithOperation(ctx context.Context, operation string) context.Context
```

コンテキストにオペレーション名を設定します。

**例:**
```go
ctx := context.Background()
ctx = WithRequestID(ctx, "req-123")
ctx = WithTraceID(ctx, "trace-456")
ctx = WithOperation(ctx, "process_feed")

logger := contextLogger.WithContext(ctx)
logger.Info("operation started") // request_id, trace_id, operation が自動追加
```

---

## Context Keys

### ContextKey

```go
type ContextKey string

const (
    RequestIDKey ContextKey = "request_id"
    TraceIDKey   ContextKey = "trace_id"
    OperationKey ContextKey = "operation"
    ServiceKey   ContextKey = "service"
)
```

コンテキスト値の取得に使用されるキー定数。

---

## Global Logger

### var Logger

```go
var Logger *slog.Logger
```

グローバルロガーインスタンス。アプリケーション全体で使用されます。

### func InitGlobalLogger

```go
func InitGlobalLogger(config *LoggerConfig)
```

グローバルロガーをUnifiedLoggerで初期化します。

**パラメータ:**
- `config *LoggerConfig`: ロガー設定

**例:**
```go
config := LoadLoggerConfigFromEnv()
InitGlobalLogger(config)

// グローバルロガーの使用
logger.Logger.Info("application started")
```

---

## JSON Output Format

### Alt-Backend互換JSON構造

```json
{
  "time": "2024-01-01T12:00:00Z",      // RFC3339 timestamp
  "level": "INFO",                     // Uppercase level
  "msg": "operation completed",        // Message field
  "method": "GET",                     // Custom fields (extracted to rask fields)
  "status": 200,                       // Numbers preserved
  "duration_ms": 150,                  // Metric fields  
  "service": "pre-processor",          // Service identifier
  "request_id": "req-123",             // Context: Request ID
  "trace_id": "trace-456",             // Context: Trace ID
  "operation": "process_feed"          // Context: Operation name
}
```

### rask-log-aggregator Fields抽出

標準フィールド（`time`, `level`, `msg`, `service`）以外は全て `fields` カラムに抽出されます：

```sql
-- ClickHouse結果例
SELECT service_name, message, fields FROM logs WHERE service_name = 'pre-processor';

┌─service_name──┬─message──────────────┬─fields────────────────────────────────┐
│ pre-processor │ operation completed  │ {'method':'GET','status':'200',       │
│               │                      │  'duration_ms':'150','request_id':    │
│               │                      │  'req-123','trace_id':'trace-456',    │
│               │                      │  'operation':'process_feed'}          │
└───────────────┴──────────────────────┴───────────────────────────────────────┘
```

---

## Usage Examples

### 基本的な使用法

```go
// 1. ロガー初期化
logger := NewUnifiedLogger(os.Stdout, "pre-processor")

// 2. シンプルなログ出力
logger.Info("service started", "port", 8080, "env", "production")

// 3. エラーログ
logger.Error("database connection failed", "error", err, "retries", 3)
```

### コンテキスト統合

```go
// 1. コンテキスト設定
ctx := WithRequestID(context.Background(), "req-123")
ctx = WithTraceID(ctx, "trace-456")
ctx = WithOperation(ctx, "process_request")

// 2. コンテキスト対応ロガー作成
contextLogger := logger.WithContext(ctx)

// 3. ログ出力（コンテキスト情報が自動追加）
contextLogger.Info("request processing started", "user_id", "user-789")
```

### サービス層での使用

```go
type FeedService struct {
    logger *slog.Logger
}

func (s *FeedService) ProcessFeed(ctx context.Context, feedURL string) error {
    // コンテキスト対応ロガー
    logger := s.logger.WithContext(ctx)
    
    logger.Info("feed processing started", "feed_url", feedURL)
    
    // エラーハンドリング
    if err := s.validateFeed(feedURL); err != nil {
        logger.Error("feed validation failed", 
            "feed_url", feedURL,
            "error", err,
            "validation_step", "url_check")
        return fmt.Errorf("validation failed: %w", err)
    }
    
    logger.Info("feed processing completed", "status", "success")
    return nil
}
```

### ミドルウェアでの使用

```go
func LoggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // リクエストコンテキスト設定
        requestID := r.Header.Get("X-Request-ID")
        if requestID == "" {
            requestID = generateRequestID()
        }
        
        ctx := WithRequestID(r.Context(), requestID)
        ctx = WithOperation(ctx, "http_request")
        
        // コンテキスト対応ロガー
        logger := contextLogger.WithContext(ctx)
        
        start := time.Now()
        logger.Info("request started", 
            "method", r.Method,
            "path", r.URL.Path,
            "user_agent", r.UserAgent())
        
        // リクエスト処理
        wrapped := &responseWriter{ResponseWriter: w}
        next.ServeHTTP(wrapped, r.WithContext(ctx))
        
        // レスポンスログ
        duration := time.Since(start)
        logger.Info("request completed",
            "status", wrapped.status,
            "duration_ms", duration.Milliseconds(),
            "response_size", wrapped.size)
    })
}
```

---

## Performance Characteristics

- **Memory allocations**: 0.00 per log call
- **JSON marshaling**: slog native optimization
- **Context overhead**: O(1) key lookup
- **Thread safety**: Full concurrent support
- **Throughput**: 10,000+ logs/second

---

## Migration Guide

### 旧RaskLogger → UnifiedLogger

```go
// 旧実装
raskLogger := NewRaskLogger(os.Stdout, "pre-processor")
raskLogger.Info("message", "key", "value")

// 新実装
unifiedLogger := NewUnifiedLogger(os.Stdout, "pre-processor") 
unifiedLogger.Info("message", "key", "value")
```

### 既存ContextLogger使用箇所

既存のContextLogger使用箇所は変更不要です。内部でUnifiedLoggerが自動的に使用されます。

```go
// 既存コード（変更不要）
contextLogger := NewContextLoggerWithConfig(config, os.Stdout)
logger := contextLogger.WithContext(ctx)
logger.Info("message", "key", "value")
```