-- OpenTelemetry Log Data Model準拠のログテーブル
-- https://opentelemetry.io/docs/specs/otel/logs/data-model/
--
-- Created: 2026-01-10
-- Purpose: Unified logging with distributed tracing support

CREATE TABLE IF NOT EXISTS otel_logs (
    -- タイムスタンプ (ナノ秒精度)
    Timestamp DateTime64(9, 'UTC') CODEC(DoubleDelta, ZSTD(1)),
    ObservedTimestamp DateTime64(9, 'UTC') CODEC(DoubleDelta, ZSTD(1)),

    -- トレースコンテキスト
    -- TraceId: 32文字の16進数 (128ビット)
    -- SpanId: 16文字の16進数 (64ビット)
    TraceId FixedString(32) CODEC(ZSTD(1)),
    SpanId FixedString(16) CODEC(ZSTD(1)),
    TraceFlags UInt8 DEFAULT 0 CODEC(ZSTD(1)),

    -- ログ重要度
    -- https://opentelemetry.io/docs/specs/otel/logs/data-model/#field-severitynumber
    SeverityText LowCardinality(String) CODEC(ZSTD(1)),
    SeverityNumber UInt8 DEFAULT 0 CODEC(ZSTD(1)),

    -- ログ本体
    Body String CODEC(ZSTD(3)),

    -- リソース属性 (サービス情報等)
    ResourceSchemaUrl String DEFAULT '' CODEC(ZSTD(1)),
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),

    -- スコープ属性 (計装ライブラリ情報)
    ScopeSchemaUrl String DEFAULT '' CODEC(ZSTD(1)),
    ScopeName String DEFAULT '' CODEC(ZSTD(1)),
    ScopeVersion String DEFAULT '' CODEC(ZSTD(1)),
    ScopeAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),

    -- ログ属性 (イベント固有の情報)
    LogAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),

    -- マテリアライズドカラム (検索最適化用)
    ServiceName LowCardinality(String)
        MATERIALIZED ResourceAttributes['service.name'] CODEC(ZSTD(1)),
    ServiceVersion LowCardinality(String)
        MATERIALIZED ResourceAttributes['service.version'] CODEC(ZSTD(1)),
    DeploymentEnvironment LowCardinality(String)
        MATERIALIZED ResourceAttributes['deployment.environment'] CODEC(ZSTD(1)),

    -- インデックス
    INDEX idx_trace_id TraceId TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_span_id SpanId TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_body Body TYPE tokenbf_v1(32768, 3, 0) GRANULARITY 1,
    INDEX idx_severity SeverityNumber TYPE minmax GRANULARITY 1
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, SeverityNumber, Timestamp)
TTL Timestamp + INTERVAL 7 DAY DELETE
SETTINGS
    index_granularity = 8192,
    ttl_only_drop_parts = 1;
