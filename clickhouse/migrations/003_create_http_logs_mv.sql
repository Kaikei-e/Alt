-- Materialized View for HTTP Logs
-- Populates http_logs from the raw edge access-log rows in `logs`.
--
-- Matches BOTH known edge-log producer shapes instead of a single
-- hardcoded one: nginx logs service_name='nginx' with http_-prefixed keys
-- (http_method/http_path/...), plecto-proxy logs service_name='plecto-proxy'
-- with bare keys (method/path/...). The nginx -> plecto-proxy cutover
-- silently killed the previous nginx-only version of this view for ~4
-- weeks because it recognized only one of these shapes; a later fix
-- swapped in plecto-proxy-only matching and would have broken the same
-- way on the *next* rollback or swap. Every downstream SLO query in
-- observability/grafana/dashboards/slo-overview.json reads FROM http_logs
-- with no service_name filter, so admitting an unqualified third shape
-- here (any row with a `method`/`path`/`status` field, from any service)
-- would silently corrupt the availability SLO -- each branch below is
-- gated on both its producer's service_name AND its field shape, not
-- shape alone. Adding a future edge-proxy requires adding a new branch
-- here; it is not zero-touch swap-proof, only two-producer-proof.
--
-- This file is replayed on every ClickHouse startup (see
-- entrypoint-wrapper.sh) with no migration-version tracking, and
-- `CREATE MATERIALIZED VIEW IF NOT EXISTS` is a permanent no-op once a view
-- of this name already exists -- it will not pick up a changed SELECT.
-- Drop and recreate unconditionally so a corrected definition here always
-- takes effect on the next restart.
DROP VIEW IF EXISTS http_logs_mv;

-- plecto-proxy emits request latency (fields['duration_ms']) but http_logs
-- had no column to hold it, and otel_http_requests_mv has no OTel
-- semantic-convention producer to source latency from instead. This ALTER
-- must run before the CREATE below (same file, same multiquery execution)
-- because `CREATE MATERIALIZED VIEW ... TO http_logs` validates its SELECT
-- against http_logs's current column set.
--
-- AFTER user_agent is required, not cosmetic: `MATERIALIZED VIEW ... TO
-- table` inserts positionally like a plain INSERT SELECT. Without AFTER,
-- ADD COLUMN appends duration_ms at the end (past container_id) on an
-- already-deployed table, which would shift service_name/container_id one
-- slot out of position against the SELECT list below and corrupt every
-- row. AFTER keeps the column position identical to a fresh install (see
-- 002_create_http_logs_table.sql), where duration_ms is already declared
-- in this same slot.
ALTER TABLE http_logs ADD COLUMN IF NOT EXISTS duration_ms Float64 DEFAULT 0 AFTER user_agent;

CREATE MATERIALIZED VIEW http_logs_mv
TO http_logs
AS
SELECT
    generateUUIDv4() AS log_id,
    timestamp,
    -- Per-producer field mapping: nginx uses http_-prefixed keys,
    -- plecto-proxy uses bare keys. The WHERE clause below guarantees
    -- exactly one branch's fields are populated for any row that reaches
    -- this SELECT, so picking by service_name here is safe.
    if(service_name = 'nginx', fields['http_method'], fields['method']) AS method,
    if(service_name = 'nginx', fields['http_path'], fields['path']) AS path,
    if(service_name = 'nginx', toUInt16OrZero(fields['http_status']), toUInt16OrZero(fields['status'])) AS status_code,
    -- Only nginx emits a response size; plecto-proxy does not.
    if(service_name = 'nginx', toUInt64OrZero(fields['http_size']), toUInt64(0)) AS response_size,
    if(service_name = 'nginx', fields['http_ip'], fields['client']) AS ip_address,
    -- Only nginx emits a user-agent; plecto-proxy does not.
    if(service_name = 'nginx', fields['http_ua'], '') AS user_agent,
    -- Only plecto-proxy emits request latency; nginx logs predate this
    -- column and never populated it.
    if(service_name = 'nginx', toFloat64(0), toFloat64OrZero(fields['duration_ms'])) AS duration_ms,
    service_name,
    container_id
FROM logs
WHERE (
    service_name = 'nginx'
    AND mapContains(fields, 'http_method')
    AND mapContains(fields, 'http_path')
    AND mapContains(fields, 'http_status')
    AND fields['http_method'] != ''
  )
  OR (
    service_name = 'plecto-proxy'
    AND mapContains(fields, 'method')
    AND mapContains(fields, 'path')
    AND mapContains(fields, 'status')
    AND fields['method'] != ''
  );
