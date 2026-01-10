-- HTTP分析用マテリアライズドビュー
-- otel_logs から HTTP 属性を抽出して専用テーブルに保存
--
-- Created: 2026-01-10
-- Purpose: Fast HTTP request analytics

-- HTTPリクエスト保存先テーブル
CREATE TABLE IF NOT EXISTS otel_http_requests (
    Timestamp DateTime64(9, 'UTC') CODEC(DoubleDelta, ZSTD(1)),
    TraceId FixedString(32) CODEC(ZSTD(1)),
    SpanId FixedString(16) CODEC(ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),

    -- HTTP属性
    HttpMethod LowCardinality(String) CODEC(ZSTD(1)),
    HttpRoute String CODEC(ZSTD(1)),
    HttpStatusCode UInt16 CODEC(ZSTD(1)),
    ResponseSize UInt64 CODEC(DoubleDelta, ZSTD(1)),
    RequestDuration Float64 CODEC(ZSTD(1)),

    -- クライアント情報
    UserId String DEFAULT '' CODEC(ZSTD(1)),
    ClientIp String DEFAULT '' CODEC(ZSTD(1)),
    UserAgent String DEFAULT '' CODEC(ZSTD(1)),

    -- インデックス
    INDEX idx_route HttpRoute TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_status HttpStatusCode TYPE minmax GRANULARITY 1
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, HttpRoute, Timestamp)
TTL Timestamp + INTERVAL 7 DAY DELETE
SETTINGS
    index_granularity = 8192,
    ttl_only_drop_parts = 1;

-- HTTPリクエスト抽出マテリアライズドビュー
CREATE MATERIALIZED VIEW IF NOT EXISTS otel_http_requests_mv
TO otel_http_requests
AS SELECT
    Timestamp,
    TraceId,
    SpanId,
    ServiceName,
    LogAttributes['http.method'] AS HttpMethod,
    LogAttributes['http.route'] AS HttpRoute,
    toUInt16OrZero(LogAttributes['http.status_code']) AS HttpStatusCode,
    toUInt64OrZero(LogAttributes['http.response.body.size']) AS ResponseSize,
    toFloat64OrZero(LogAttributes['http.request.duration']) AS RequestDuration,
    LogAttributes['user.id'] AS UserId,
    LogAttributes['http.client_ip'] AS ClientIp,
    LogAttributes['http.user_agent'] AS UserAgent
FROM otel_logs
WHERE LogAttributes['http.method'] != '';

-- エラーログ保存先テーブル
CREATE TABLE IF NOT EXISTS otel_error_logs (
    Timestamp DateTime64(9, 'UTC') CODEC(DoubleDelta, ZSTD(1)),
    TraceId FixedString(32) CODEC(ZSTD(1)),
    SpanId FixedString(16) CODEC(ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    SeverityText LowCardinality(String) CODEC(ZSTD(1)),
    Body String CODEC(ZSTD(3)),
    ExceptionType String DEFAULT '' CODEC(ZSTD(1)),
    ExceptionMessage String DEFAULT '' CODEC(ZSTD(1)),
    Stacktrace String DEFAULT '' CODEC(ZSTD(3)),

    INDEX idx_trace_id TraceId TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_exception_type ExceptionType TYPE bloom_filter(0.01) GRANULARITY 1
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, Timestamp)
TTL Timestamp + INTERVAL 14 DAY DELETE
SETTINGS
    index_granularity = 8192,
    ttl_only_drop_parts = 1;

-- エラーログ抽出マテリアライズドビュー (SeverityNumber >= 17 = ERROR以上)
CREATE MATERIALIZED VIEW IF NOT EXISTS otel_error_logs_mv
TO otel_error_logs
AS SELECT
    Timestamp,
    TraceId,
    SpanId,
    ServiceName,
    SeverityText,
    Body,
    LogAttributes['exception.type'] AS ExceptionType,
    LogAttributes['exception.message'] AS ExceptionMessage,
    LogAttributes['exception.stacktrace'] AS Stacktrace
FROM otel_logs
WHERE SeverityNumber >= 17;
