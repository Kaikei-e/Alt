-- OpenTelemetry Span Data Model準拠のトレーステーブル
-- https://opentelemetry.io/docs/specs/otel/trace/api/#span
--
-- Created: 2026-01-10
-- Purpose: Distributed tracing storage

CREATE TABLE IF NOT EXISTS otel_traces (
    -- タイムスタンプ
    Timestamp DateTime64(9, 'UTC') CODEC(DoubleDelta, ZSTD(1)),

    -- トレースコンテキスト
    TraceId FixedString(32) CODEC(ZSTD(1)),
    SpanId FixedString(16) CODEC(ZSTD(1)),
    ParentSpanId FixedString(16) DEFAULT '' CODEC(ZSTD(1)),
    TraceState String DEFAULT '' CODEC(ZSTD(1)),

    -- スパン情報
    SpanName LowCardinality(String) CODEC(ZSTD(1)),
    SpanKind Enum8(
        'UNSPECIFIED' = 0,
        'INTERNAL' = 1,
        'SERVER' = 2,
        'CLIENT' = 3,
        'PRODUCER' = 4,
        'CONSUMER' = 5
    ) DEFAULT 'UNSPECIFIED' CODEC(ZSTD(1)),

    -- サービス情報
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),

    -- 属性
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    SpanAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),

    -- パフォーマンス (ナノ秒)
    Duration Int64 CODEC(DoubleDelta, ZSTD(1)),

    -- ステータス
    StatusCode Enum8(
        'UNSET' = 0,
        'OK' = 1,
        'ERROR' = 2
    ) DEFAULT 'UNSET' CODEC(ZSTD(1)),
    StatusMessage String DEFAULT '' CODEC(ZSTD(1)),

    -- イベント・リンク (JSON配列)
    Events String DEFAULT '[]' CODEC(ZSTD(3)),
    Links String DEFAULT '[]' CODEC(ZSTD(3)),

    -- マテリアライズドカラム
    DurationMs Float64 MATERIALIZED Duration / 1000000.0,

    -- インデックス
    INDEX idx_trace_id TraceId TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_parent_span_id ParentSpanId TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_span_name SpanName TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_duration Duration TYPE minmax GRANULARITY 1,
    INDEX idx_status StatusCode TYPE set(3) GRANULARITY 1
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, Timestamp, TraceId)
TTL Timestamp + INTERVAL 7 DAY DELETE
SETTINGS
    index_granularity = 8192,
    ttl_only_drop_parts = 1;
