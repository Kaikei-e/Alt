-- Materialized View for HTTP Logs
-- Automatically populates http_logs table when HTTP-related logs are inserted into logs table
-- Triggered on INSERT to logs table for nginx service with http_method field

CREATE MATERIALIZED VIEW IF NOT EXISTS http_logs_mv
TO http_logs
AS
SELECT
    generateUUIDv4() AS log_id,
    timestamp,
    fields['http_method'] AS method,
    fields['http_path'] AS path,
    toUInt16OrZero(fields['http_status']) AS status_code,
    toUInt64OrZero(fields['http_size']) AS response_size,
    fields['http_ip'] AS ip_address,
    fields['http_ua'] AS user_agent,
    service_name,
    container_id
FROM logs
WHERE service_name = 'nginx'
  AND mapContains(fields, 'http_method')
  AND fields['http_method'] != '';
