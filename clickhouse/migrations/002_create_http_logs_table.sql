-- HTTP Logs Table
-- Stores structured HTTP request data extracted from nginx and other HTTP services
-- Linked to the main logs table via timestamp + container_id

CREATE TABLE IF NOT EXISTS http_logs (
    -- Primary identifier
    log_id UUID DEFAULT generateUUIDv4(),

    -- Timestamp (for correlation with logs table)
    timestamp DateTime64(3, 'UTC'),

    -- HTTP request fields
    method LowCardinality(String),
    path String,
    status_code UInt16,
    response_size UInt64,

    -- Client information
    ip_address String,
    user_agent String,

    -- Service metadata
    service_name LowCardinality(String),
    container_id String

) ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (service_name, status_code, timestamp)
TTL timestamp + INTERVAL 2 DAY DELETE
SETTINGS ttl_only_drop_parts = 1;
