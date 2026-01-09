CREATE TABLE IF NOT EXISTS logs (
    service_type LowCardinality(String),
    log_type LowCardinality(String),
    message String,
    level Enum8('Debug' = 0, 'Info' = 1, 'Warn' = 2, 'Error' = 3, 'Fatal' = 4),
    timestamp DateTime64(3, 'UTC'),
    stream LowCardinality(String),
    container_id String,
    service_name LowCardinality(String),
    service_group LowCardinality(String),
    fields Map(String, String)
) ENGINE = MergeTree()
PARTITION BY (service_group, service_name)
ORDER BY (timestamp)
TTL timestamp + INTERVAL 2 DAY DELETE
SETTINGS ttl_only_drop_parts = 1;
