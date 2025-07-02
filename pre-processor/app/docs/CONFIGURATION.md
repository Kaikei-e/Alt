# Logging Configuration Guide

## 概要

Pre-processorサービスのロギング設定に関する包括的なガイドです。UnifiedLoggerを使用したシンプルで効率的な設定方法を説明します。

---

## 🔧 環境変数

### 必須設定

| 変数名 | デフォルト値 | 説明 | 例 |
|--------|-------------|------|-----|
| `LOG_LEVEL` | `info` | ログレベル | `debug`, `info`, `warn`, `error` |
| `SERVICE_NAME` | `pre-processor` | サービス識別子 | `pre-processor`, `pre-processor-dev` |

### 廃止された設定

| 変数名 | 状態 | 代替 |
|--------|------|------|
| `USE_RASK_LOGGER` | 廃止 | 常にUnifiedLogger使用 |
| `LOG_FORMAT` | 廃止 | 常にJSON形式 |

---

## 📋 設定例

### Docker Compose設定

```yaml
# compose.yaml
services:
  pre-processor:
    build:
      context: ./pre-processor
      dockerfile: Dockerfile.preprocess
    environment:
      # データベース設定
      - DB_HOST=db
      - DB_PORT=5432
      - DB_NAME=${DB_NAME}
      - PRE_PROCESSOR_DB_USER=${PRE_PROCESSOR_DB_USER}
      - PRE_PROCESSOR_DB_PASSWORD=${PRE_PROCESSOR_DB_PASSWORD}
      
      # ロギング設定（簡略化）
      - LOG_LEVEL=info
      - SERVICE_NAME=pre-processor
      # USE_RASK_LOGGER削除 - unified on slog
      
    ports:
      - "9200:9200"
    networks:
      - alt-network
    depends_on:
      db:
        condition: service_healthy
    restart: always
```

### 環境ファイル設定

```bash
# .env
LOG_LEVEL=info
SERVICE_NAME=pre-processor

# 開発環境用
# LOG_LEVEL=debug
# SERVICE_NAME=pre-processor-dev
```

### Dockerfile設定

```dockerfile
# Dockerfile.preprocess
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY . .
RUN go build -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

# デフォルト環境変数
ENV LOG_LEVEL=info
ENV SERVICE_NAME=pre-processor

COPY --from=builder /app/main .
CMD ["./main"]
```

---

## 🏗️ アプリケーション設定

### 基本初期化

```go
// main.go
package main

import (
    "context"
    "os"
    
    logger "pre-processor/utils/logger"
)

func main() {
    // 1. 設定読み込み
    config := logger.LoadLoggerConfigFromEnv()
    
    // 2. ContextLogger初期化
    contextLogger := logger.NewContextLoggerWithConfig(config, os.Stdout)
    
    // 3. グローバルロガー設定
    logger.Logger = contextLogger.WithContext(context.Background())
    
    // 4. 起動ログ
    logger.Logger.Info("Pre-processor service starting",
        "log_level", config.Level,
        "service_name", config.ServiceName,
        "version", "1.0.0")
    
    // アプリケーション処理...
}
```

### 設定構造体

```go
// utils/logger/config.go
type LoggerConfig struct {
    Level       string // 環境変数: LOG_LEVEL
    Format      string // 廃止（常に"json"）
    ServiceName string // 環境変数: SERVICE_NAME
    UseRask     bool   // 廃止（常にfalse）
}

// 環境変数から設定読み込み
func LoadLoggerConfigFromEnv() *LoggerConfig {
    return &LoggerConfig{
        Level:       getEnvOrDefault("LOG_LEVEL", "info"),
        Format:      "json", // 固定
        ServiceName: getEnvOrDefault("SERVICE_NAME", "pre-processor"),
        UseRask:     false, // 固定
    }
}
```

---

## 📈 環境別設定

### 開発環境

```yaml
# compose.dev.yaml
services:
  pre-processor:
    environment:
      - LOG_LEVEL=debug      # 詳細ログ
      - SERVICE_NAME=pre-processor-dev
    volumes:
      - ./logs:/app/logs     # ローカルログファイル（オプション）
```

```go
// 開発環境専用設定
if os.Getenv("ENV") == "development" {
    config.Level = "debug"
    
    // 開発用の追加ログ
    logger.Logger.Debug("Development mode enabled",
        "debug_features", []string{"verbose_logging", "performance_metrics"})
}
```

### 本番環境

```yaml
# compose.prod.yaml
services:
  pre-processor:
    environment:
      - LOG_LEVEL=info       # 標準ログレベル
      - SERVICE_NAME=pre-processor
    deploy:
      resources:
        limits:
          memory: 512M       # メモリ制限
          cpus: '1'          # CPU制限
```

```go
// 本番環境最適化
if os.Getenv("ENV") == "production" {
    // エラー時のみ詳細ログ
    if config.Level == "debug" {
        config.Level = "info" // 強制的にINFOレベル
        logger.Logger.Warn("Debug level not allowed in production, using info level")
    }
}
```

### ステージング環境

```yaml
# compose.staging.yaml
services:
  pre-processor:
    environment:
      - LOG_LEVEL=debug      # テスト用詳細ログ
      - SERVICE_NAME=pre-processor-staging
```

---

## 🎛️ ログレベル詳細設定

### ログレベル別出力例

```go
logger := logger.NewUnifiedLogger(os.Stdout, "pre-processor")

// DEBUG: 開発・デバッグ用
logger.Debug("Processing item", 
    "item_id", "item-123",
    "processing_step", "validation",
    "item_data", itemData) // 詳細データ含む

// INFO: 標準的な動作情報  
logger.Info("Request completed",
    "method", "GET",
    "path", "/api/feeds",
    "status", 200,
    "duration_ms", 150)

// WARN: 注意が必要だが処理は継続
logger.Warn("Rate limit approaching",
    "current_requests", 95,
    "limit", 100,
    "time_window", "1m")

// ERROR: エラー発生、調査が必要
logger.Error("Database connection failed",
    "error", err,
    "connection_string", "postgres://...",
    "retry_count", 3)
```

### レベル別設定推奨事項

| レベル | 用途 | 含める情報 | パフォーマンス影響 |
|--------|------|------------|-------------------|
| `debug` | 開発・デバッグ | 詳細なデータ、処理ステップ | 高（本番では避ける） |
| `info` | 通常運用 | リクエスト/レスポンス、重要な状態変化 | 低 |
| `warn` | 監視・アラート | 異常状態、制限値接近 | 低 |
| `error` | 障害対応 | エラー詳細、スタックトレース | 低 |

---

## 🔄 動的設定変更

### ランタイム設定変更（今後実装予定）

```go
// 将来的な機能（現在は未実装）
type DynamicConfig struct {
    LogLevel string `json:"log_level"`
}

// HTTP エンドポイントでレベル変更
func handleConfigUpdate(w http.ResponseWriter, r *http.Request) {
    var config DynamicConfig
    json.NewDecoder(r.Body).Decode(&config)
    
    // ロガーレベル動的変更
    updateLogLevel(config.LogLevel)
    
    logger.Logger.Info("Log level updated", "new_level", config.LogLevel)
}
```

### 設定ホットリロード（計画中）

```bash
# シグナルベースの設定リロード
kill -SIGUSR1 $(pgrep pre-processor)

# または REST API経由
curl -X POST http://localhost:9200/admin/config \
  -H "Content-Type: application/json" \
  -d '{"log_level": "debug"}'
```

---

## 🧪 設定テスト

### 設定検証スクリプト

```bash
#!/bin/bash
# validate_config.sh

echo "=== Pre-processor Logging Configuration Test ==="

# 1. 環境変数確認
echo "1. Checking environment variables..."
echo "LOG_LEVEL: ${LOG_LEVEL:-not set}"
echo "SERVICE_NAME: ${SERVICE_NAME:-not set}"

# 2. ログレベル検証
echo "2. Testing log levels..."
for level in debug info warn error; do
    echo "Testing level: $level"
    LOG_LEVEL=$level docker run --rm pre-processor:latest \
        sh -c 'echo "Test log for level: $LOG_LEVEL"' 2>/dev/null && \
        echo "✅ $level works" || echo "❌ $level failed"
done

# 3. JSON形式確認
echo "3. Checking JSON format..."
LOG_OUTPUT=$(LOG_LEVEL=info docker run --rm pre-processor:latest \
    sh -c 'timeout 5s ./main' 2>&1 | head -n 1)

echo "$LOG_OUTPUT" | jq . >/dev/null 2>&1 && \
    echo "✅ Valid JSON output" || echo "❌ Invalid JSON output"

echo "=== Test Complete ==="
```

### 単体テスト

```go
// config_test.go
func TestLoggerConfigFromEnv(t *testing.T) {
    tests := map[string]struct {
        envVars        map[string]string
        expectedLevel  string
        expectedService string
    }{
        "default values": {
            envVars:         map[string]string{},
            expectedLevel:   "info",
            expectedService: "pre-processor",
        },
        "custom values": {
            envVars: map[string]string{
                "LOG_LEVEL":    "debug",
                "SERVICE_NAME": "test-service",
            },
            expectedLevel:   "debug", 
            expectedService: "test-service",
        },
    }

    for name, tc := range tests {
        t.Run(name, func(t *testing.T) {
            // 環境変数設定
            for key, value := range tc.envVars {
                os.Setenv(key, value)
                defer os.Unsetenv(key)
            }

            config := LoadLoggerConfigFromEnv()
            
            assert.Equal(t, tc.expectedLevel, config.Level)
            assert.Equal(t, tc.expectedService, config.ServiceName)
        })
    }
}
```

---

## 🚀 最適化設定

### パフォーマンス最適化

```go
// 高負荷環境での設定
type HighPerformanceConfig struct {
    LogLevel    string
    BufferSize  int
    FlushInterval time.Duration
}

func NewHighPerformanceLogger() *UnifiedLogger {
    // バッファリング設定（将来実装）
    logger := NewUnifiedLogger(os.Stdout, "pre-processor")
    
    // 本番環境ではERRORレベル以上のみ
    if os.Getenv("ENV") == "production" {
        // 現在は環境変数でのみ制御
        // 将来的にはプログラマティックな制御を追加
    }
    
    return logger
}
```

### メモリ使用量最適化

```yaml
# compose.yaml - リソース制限
services:
  pre-processor:
    environment:
      - LOG_LEVEL=warn  # 必要最小限のログ
    deploy:
      resources:
        limits:
          memory: 256M    # メモリ制限
        reservations:
          memory: 128M    # 予約メモリ
```

### ネットワーク最適化

```yaml
# ログ転送最適化
services:
  pre-processor:
    logging:
      driver: "json-file"
      options:
        max-size: "10m"     # ログファイルサイズ制限
        max-file: "3"       # ローテーション数
        compress: "true"    # 圧縮有効
```

---

## 📊 監視・メトリクス設定

### ログ監視設定

```yaml
# monitoring/logging-alerts.yaml
groups:
  - name: logging-alerts
    rules:
      - alert: HighErrorRate
        expr: |
          rate(logs_total{service="pre-processor",level="error"}[5m]) > 0.1
        for: 2m
        annotations:
          summary: "High error rate in pre-processor logs"
          
      - alert: LoggingVolumeHigh
        expr: |
          rate(logs_total{service="pre-processor"}[5m]) > 100
        for: 5m
        annotations:
          summary: "High logging volume detected"
```

### ヘルスチェック設定

```go
// health_check.go
type LoggingHealth struct {
    LastLogTime time.Time `json:"last_log_time"`
    LogCount    int64     `json:"log_count_1h"`
    ErrorRate   float64   `json:"error_rate_1h"`
}

func (h *HealthHandler) GetLoggingHealth() LoggingHealth {
    // ログ統計の収集
    return LoggingHealth{
        LastLogTime: time.Now(),
        LogCount:    h.logCounter.Get(),
        ErrorRate:   h.calculateErrorRate(),
    }
}
```

---

## 🔒 セキュリティ設定

### 機密情報のフィルタリング

```go
// セキュアログ設定
func secureLogger() *UnifiedLogger {
    logger := NewUnifiedLogger(os.Stdout, "pre-processor")
    
    // 機密情報フィルタリング（将来実装）
    // password, token, secretなどの自動マスキング
    return logger
}

// ログ時の注意事項
func (s *Service) ProcessAuth(ctx context.Context, token string) error {
    logger := s.logger.WithContext(ctx)
    
    // ❌ 機密情報をログに含めない
    // logger.Info("processing auth", "token", token)
    
    // ✅ 安全な情報のみログ出力
    logger.Info("processing auth", 
        "token_length", len(token),
        "token_prefix", token[:4]+"***")
}
```

### アクセス制御

```yaml
# セキュリティ設定
services:
  pre-processor:
    security_opt:
      - no-new-privileges:true
    read_only: true
    tmpfs:
      - /tmp:noexec,nosuid,size=128m
    environment:
      - LOG_LEVEL=info  # 機密情報を含むdebugは本番で無効
```

---

## 📚 設定リファレンス

### 完全な環境変数リスト

| カテゴリ | 変数名 | デフォルト値 | 必須 | 説明 |
|----------|--------|-------------|------|------|
| **ログ** | `LOG_LEVEL` | `info` | No | `debug`/`info`/`warn`/`error` |
| **ログ** | `SERVICE_NAME` | `pre-processor` | No | サービス識別子 |
| **廃止** | `USE_RASK_LOGGER` | - | No | 廃止：常にUnifiedLogger |
| **廃止** | `LOG_FORMAT` | - | No | 廃止：常にJSON |

### 設定ファイルテンプレート

```yaml
# compose.yaml template
services:
  pre-processor:
    environment:
      # === Required Configuration ===
      - LOG_LEVEL=${LOG_LEVEL:-info}
      - SERVICE_NAME=${SERVICE_NAME:-pre-processor}
      
      # === Database Configuration ===
      - DB_HOST=db
      - DB_PORT=5432
      - DB_NAME=${DB_NAME}
      - PRE_PROCESSOR_DB_USER=${PRE_PROCESSOR_DB_USER}
      - PRE_PROCESSOR_DB_PASSWORD=${PRE_PROCESSOR_DB_PASSWORD}
      
    # === Resource Limits ===
    deploy:
      resources:
        limits:
          memory: 512M
          cpus: '1'
          
    # === Health Check ===
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9200/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      
    # === Logging Configuration ===
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```