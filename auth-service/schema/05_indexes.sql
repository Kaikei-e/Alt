-- ==========================================
-- 05_indexes.sql
-- 追加インデックスとパフォーマンス最適化
-- ==========================================

-- 複合インデックス（クエリパフォーマンス向上）

-- ユーザー関連の複合インデックス
CREATE INDEX IF NOT EXISTS idx_users_tenant_status_role ON users(tenant_id, status, role) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_email_status ON users(email, status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_kratos_status ON users(kratos_identity_id, status) WHERE deleted_at IS NULL;

-- セッション関連の複合インデックス
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_active_expires ON user_sessions(user_id, active, expires_at) WHERE active = true;
CREATE INDEX IF NOT EXISTS idx_user_sessions_kratos_expires ON user_sessions(kratos_session_id, expires_at) WHERE active = true;
CREATE INDEX IF NOT EXISTS idx_user_sessions_ip_activity ON user_sessions(ip_address, last_activity_at DESC);

-- 監査ログ関連の複合インデックス
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_action_date ON audit_logs(tenant_id, action, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_action_date ON audit_logs(user_id, action, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource_date ON audit_logs(resource_type, resource_id, created_at DESC);

-- 部分インデックス（ストレージ効率化）

-- アクティブなセッションのみ
CREATE INDEX IF NOT EXISTS idx_active_user_sessions ON user_sessions(user_id, last_activity_at DESC) WHERE active = true AND expires_at > CURRENT_TIMESTAMP;

-- 未削除のユーザー・テナントのみ
CREATE INDEX IF NOT EXISTS idx_active_users ON users(tenant_id, created_at DESC) WHERE deleted_at IS NULL AND status = 'active';
CREATE INDEX IF NOT EXISTS idx_active_tenants ON tenants(status, created_at DESC) WHERE deleted_at IS NULL AND status = 'active';

-- 期限内のCSRFトークンのみ
CREATE INDEX IF NOT EXISTS idx_valid_csrf_tokens ON csrf_tokens(session_id, user_id) WHERE expires_at > CURRENT_TIMESTAMP;

-- GINインデックス（JSON検索高速化）

-- テナント設定の JSON 検索
CREATE INDEX IF NOT EXISTS idx_tenant_settings_gin ON tenants USING GIN(settings) WHERE deleted_at IS NULL;

-- ユーザー設定の JSON 検索
CREATE INDEX IF NOT EXISTS idx_user_preferences_gin ON users USING GIN(preferences) WHERE deleted_at IS NULL;

-- ユーザー設定詳細の JSON 検索
CREATE INDEX IF NOT EXISTS idx_user_preferences_value_gin ON user_preferences USING GIN(value);

-- セッションデバイス情報の JSON 検索
CREATE INDEX IF NOT EXISTS idx_session_device_gin ON user_sessions USING GIN(device_info) WHERE active = true;

-- 監査ログ詳細の JSON 検索
CREATE INDEX IF NOT EXISTS idx_audit_details_gin ON audit_logs USING GIN(details);

-- 関数ベースインデックス

-- メールアドレスの小文字化検索
CREATE INDEX IF NOT EXISTS idx_users_email_lower ON users(lower(email)) WHERE deleted_at IS NULL;

-- テナントスラッグの小文字化検索
CREATE INDEX IF NOT EXISTS idx_tenants_slug_lower ON tenants(lower(slug)) WHERE deleted_at IS NULL;

-- 日付の年月別インデックス
CREATE INDEX IF NOT EXISTS idx_users_created_year_month ON users(EXTRACT(YEAR FROM created_at), EXTRACT(MONTH FROM created_at));
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_year_month ON audit_logs(EXTRACT(YEAR FROM created_at), EXTRACT(MONTH FROM created_at));

-- BRIN インデックス（大量データの範囲検索用）

-- 監査ログの作成日時（パーティション対応）
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_brin ON audit_logs USING BRIN(created_at) WITH (pages_per_range = 128);

-- セッションの作成日時・活動日時
CREATE INDEX IF NOT EXISTS idx_user_sessions_created_brin ON user_sessions USING BRIN(created_at) WITH (pages_per_range = 128);
CREATE INDEX IF NOT EXISTS idx_user_sessions_activity_brin ON user_sessions USING BRIN(last_activity_at) WITH (pages_per_range = 128);

-- 高頻度クエリ用の最適化インデックス

-- ログイン認証用
CREATE INDEX IF NOT EXISTS idx_auth_lookup ON users(kratos_identity_id, status, tenant_id) WHERE deleted_at IS NULL;

-- セッション検証用
CREATE INDEX IF NOT EXISTS idx_session_validation ON user_sessions(kratos_session_id, active, expires_at, user_id);

-- CSRF検証用
CREATE INDEX IF NOT EXISTS idx_csrf_validation ON csrf_tokens(token, session_id, expires_at, user_id);

-- テナント管理用
CREATE INDEX IF NOT EXISTS idx_tenant_management ON tenants(slug, status) WHERE deleted_at IS NULL;

-- ユーザー一覧表示用
CREATE INDEX IF NOT EXISTS idx_user_listing ON users(tenant_id, status, created_at DESC) WHERE deleted_at IS NULL;

-- 監査ログ検索用
CREATE INDEX IF NOT EXISTS idx_audit_search ON audit_logs(tenant_id, user_id, action, created_at DESC);

-- カバリングインデックス（include columns for query performance）

-- PostgreSQL 11+ のINCLUDE clause を使用
-- ユーザー詳細取得用
CREATE INDEX IF NOT EXISTS idx_users_lookup_covering 
ON users(kratos_identity_id) 
INCLUDE (id, tenant_id, email, name, role, status, preferences, last_login_at)
WHERE deleted_at IS NULL;

-- セッション詳細取得用
CREATE INDEX IF NOT EXISTS idx_sessions_lookup_covering 
ON user_sessions(kratos_session_id) 
INCLUDE (id, user_id, device_info, ip_address, active, expires_at, last_activity_at)
WHERE active = true;

-- テナント詳細取得用
CREATE INDEX IF NOT EXISTS idx_tenants_lookup_covering 
ON tenants(slug) 
INCLUDE (id, name, status, settings, created_at, updated_at)
WHERE deleted_at IS NULL;

-- 統計情報更新用のカスタム関数

-- テーブル統計の手動更新
CREATE OR REPLACE FUNCTION update_table_statistics()
RETURNS VOID AS $$
BEGIN
    ANALYZE tenants;
    ANALYZE users;
    ANALYZE user_sessions;
    ANALYZE csrf_tokens;
    ANALYZE user_preferences;
    ANALYZE audit_logs;
    
    RAISE NOTICE 'Table statistics updated';
END;
$$ LANGUAGE plpgsql;

-- インデックス使用状況の監視
CREATE OR REPLACE FUNCTION get_index_usage_stats()
RETURNS TABLE(
    schemaname TEXT,
    tablename TEXT,
    indexname TEXT,
    idx_tup_read BIGINT,
    idx_tup_fetch BIGINT,
    idx_scan BIGINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        s.schemaname::TEXT,
        s.relname::TEXT as tablename,
        s.indexrelname::TEXT as indexname,
        s.idx_tup_read,
        s.idx_tup_fetch,
        s.idx_scan
    FROM pg_stat_user_indexes s
    WHERE s.schemaname = 'public'
        AND (s.relname IN ('tenants', 'users', 'user_sessions', 'csrf_tokens', 'user_preferences')
             OR s.relname LIKE 'audit_logs%')
    ORDER BY s.idx_scan DESC;
END;
$$ LANGUAGE plpgsql;

-- 未使用インデックスの検出
CREATE OR REPLACE FUNCTION find_unused_indexes()
RETURNS TABLE(
    schemaname TEXT,
    tablename TEXT,
    indexname TEXT,
    index_size TEXT
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        s.schemaname::TEXT,
        s.relname::TEXT as tablename,
        s.indexrelname::TEXT as indexname,
        pg_size_pretty(pg_relation_size(s.indexrelid))::TEXT as index_size
    FROM pg_stat_user_indexes s
    JOIN pg_index i ON s.indexrelid = i.indexrelid
    WHERE s.idx_scan = 0
        AND NOT i.indisunique
        AND NOT i.indisprimary
        AND s.schemaname = 'public'
        AND (s.relname IN ('tenants', 'users', 'user_sessions', 'csrf_tokens', 'user_preferences')
             OR s.relname LIKE 'audit_logs%')
    ORDER BY pg_relation_size(s.indexrelid) DESC;
END;
$$ LANGUAGE plpgsql;

-- インデックスサイズ監視
CREATE OR REPLACE FUNCTION get_index_sizes()
RETURNS TABLE(
    schemaname TEXT,
    tablename TEXT,
    indexname TEXT,
    index_size TEXT,
    table_size TEXT,
    ratio NUMERIC
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        s.schemaname::TEXT,
        s.relname::TEXT as tablename,
        s.indexrelname::TEXT as indexname,
        pg_size_pretty(pg_relation_size(s.indexrelid))::TEXT as index_size,
        pg_size_pretty(pg_relation_size(s.relid))::TEXT as table_size,
        ROUND((pg_relation_size(s.indexrelid)::NUMERIC / NULLIF(pg_relation_size(s.relid), 0)) * 100, 2) as ratio
    FROM pg_stat_user_indexes s
    WHERE s.schemaname = 'public'
        AND (s.relname IN ('tenants', 'users', 'user_sessions', 'csrf_tokens', 'user_preferences')
             OR s.relname LIKE 'audit_logs%')
    ORDER BY pg_relation_size(s.indexrelid) DESC;
END;
$$ LANGUAGE plpgsql;

-- 自動統計更新の設定推奨値
-- これらの設定は postgresql.conf で設定することを推奨

/*
推奨設定:

# 自動統計収集の設定
track_activities = on
track_counts = on
track_io_timing = on
track_functions = pl

# 自動VACUUM/ANALYZEの設定
autovacuum = on
autovacuum_naptime = 1min
autovacuum_vacuum_threshold = 50
autovacuum_analyze_threshold = 50
autovacuum_vacuum_scale_factor = 0.2
autovacuum_analyze_scale_factor = 0.1

# 統計対象の設定
default_statistics_target = 100
log_autovacuum_min_duration = 0

# パフォーマンス関連
shared_preload_libraries = 'pg_stat_statements'
pg_stat_statements.track = all
*/

-- コメント追加
COMMENT ON FUNCTION update_table_statistics() IS 'テーブル統計情報を手動更新';
COMMENT ON FUNCTION get_index_usage_stats() IS 'インデックス使用状況統計を取得';
COMMENT ON FUNCTION find_unused_indexes() IS '未使用インデックスを検出';
COMMENT ON FUNCTION get_index_sizes() IS 'インデックスサイズ情報を取得';