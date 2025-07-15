-- ==========================================
-- 04_audit.sql
-- 監査ログと設定管理テーブル作成
-- ==========================================

-- 監査ログテーブル (パーティション対応)
CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100),
    resource_id VARCHAR(255),
    details JSONB DEFAULT '{}',
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL,
    
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- 現在の月のパーティション作成
CREATE TABLE IF NOT EXISTS audit_logs_y2025m01 PARTITION OF audit_logs
FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');

CREATE TABLE IF NOT EXISTS audit_logs_y2025m02 PARTITION OF audit_logs
FOR VALUES FROM ('2025-02-01') TO ('2025-03-01');

CREATE TABLE IF NOT EXISTS audit_logs_y2025m03 PARTITION OF audit_logs
FOR VALUES FROM ('2025-03-01') TO ('2025-04-01');

-- インデックス (各パーティション用)
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_id ON audit_logs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at);

-- ユーザー設定詳細テーブル
CREATE TABLE IF NOT EXISTS user_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category VARCHAR(100) NOT NULL,
    key VARCHAR(100) NOT NULL,
    value JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(user_id, category, key)
);

-- インデックス
CREATE INDEX IF NOT EXISTS idx_user_preferences_user_id ON user_preferences(user_id);
CREATE INDEX IF NOT EXISTS idx_user_preferences_category ON user_preferences(category);
CREATE INDEX IF NOT EXISTS idx_user_preferences_key ON user_preferences(key);

-- updated_at の自動更新トリガー
DROP TRIGGER IF EXISTS update_user_preferences_updated_at ON user_preferences;
CREATE TRIGGER update_user_preferences_updated_at
    BEFORE UPDATE ON user_preferences
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- 自動パーティション作成関数
CREATE OR REPLACE FUNCTION create_monthly_partition(table_name TEXT, start_date DATE)
RETURNS VOID AS $$
DECLARE
    partition_name TEXT;
    end_date DATE;
    sql_command TEXT;
BEGIN
    partition_name := table_name || '_y' || EXTRACT(YEAR FROM start_date) || 'm' || LPAD(EXTRACT(MONTH FROM start_date)::TEXT, 2, '0');
    end_date := start_date + INTERVAL '1 month';

    sql_command := format('CREATE TABLE IF NOT EXISTS %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
                         partition_name, table_name, start_date, end_date);
    
    EXECUTE sql_command;
    
    -- パーティション用インデックス作成
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%s_tenant_id ON %I(tenant_id)', partition_name, partition_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%s_user_id ON %I(user_id)', partition_name, partition_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%s_action ON %I(action)', partition_name, partition_name);
    
    RAISE NOTICE 'Created partition: %', partition_name;
END;
$$ LANGUAGE plpgsql;

-- パーティション自動作成 (次の3ヶ月分)
DO $$
DECLARE
    current_month DATE;
    i INTEGER;
BEGIN
    current_month := date_trunc('month', CURRENT_DATE);
    
    FOR i IN 1..3 LOOP
        current_month := current_month + INTERVAL '1 month';
        BEGIN
            PERFORM create_monthly_partition('audit_logs', current_month);
        EXCEPTION WHEN duplicate_table THEN
            CONTINUE;
        END;
    END LOOP;
END $$;

-- 監査ログ記録関数
CREATE OR REPLACE FUNCTION log_audit_event(
    p_tenant_id UUID,
    p_user_id UUID,
    p_action VARCHAR(100),
    p_resource_type VARCHAR(100) DEFAULT NULL,
    p_resource_id VARCHAR(255) DEFAULT NULL,
    p_details JSONB DEFAULT '{}',
    p_ip_address INET DEFAULT NULL,
    p_user_agent TEXT DEFAULT NULL
)
RETURNS UUID AS $$
DECLARE
    log_id UUID;
BEGIN
    log_id := gen_random_uuid();
    
    INSERT INTO audit_logs (
        id, tenant_id, user_id, action, resource_type, resource_id,
        details, ip_address, user_agent, created_at
    ) VALUES (
        log_id, p_tenant_id, p_user_id, p_action, p_resource_type, p_resource_id,
        p_details, p_ip_address, p_user_agent, CURRENT_TIMESTAMP
    );
    
    RETURN log_id;
END;
$$ LANGUAGE plpgsql;

-- 監査ログクエリ関数
CREATE OR REPLACE FUNCTION get_audit_logs(
    p_tenant_id UUID DEFAULT NULL,
    p_user_id UUID DEFAULT NULL,
    p_action VARCHAR(100) DEFAULT NULL,
    p_start_date TIMESTAMP WITH TIME ZONE DEFAULT NULL,
    p_end_date TIMESTAMP WITH TIME ZONE DEFAULT NULL,
    p_limit INTEGER DEFAULT 100
)
RETURNS TABLE(
    id UUID,
    tenant_id UUID,
    user_id UUID,
    action VARCHAR(100),
    resource_type VARCHAR(100),
    resource_id VARCHAR(255),
    details JSONB,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMP WITH TIME ZONE
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        a.id, a.tenant_id, a.user_id, a.action, a.resource_type, a.resource_id,
        a.details, a.ip_address, a.user_agent, a.created_at
    FROM audit_logs a
    WHERE 
        (p_tenant_id IS NULL OR a.tenant_id = p_tenant_id)
        AND (p_user_id IS NULL OR a.user_id = p_user_id)
        AND (p_action IS NULL OR a.action = p_action)
        AND (p_start_date IS NULL OR a.created_at >= p_start_date)
        AND (p_end_date IS NULL OR a.created_at <= p_end_date)
    ORDER BY a.created_at DESC
    LIMIT p_limit;
END;
$$ LANGUAGE plpgsql;

-- ユーザー設定管理関数

-- 設定取得
CREATE OR REPLACE FUNCTION get_user_preference(
    p_user_id UUID,
    p_category VARCHAR(100),
    p_key VARCHAR(100)
)
RETURNS JSONB AS $$
DECLARE
    pref_value JSONB;
BEGIN
    SELECT value INTO pref_value
    FROM user_preferences
    WHERE user_id = p_user_id
        AND category = p_category
        AND key = p_key;
    
    RETURN COALESCE(pref_value, 'null'::JSONB);
END;
$$ LANGUAGE plpgsql;

-- 設定更新
CREATE OR REPLACE FUNCTION set_user_preference(
    p_user_id UUID,
    p_category VARCHAR(100),
    p_key VARCHAR(100),
    p_value JSONB
)
RETURNS VOID AS $$
BEGIN
    INSERT INTO user_preferences (user_id, category, key, value)
    VALUES (p_user_id, p_category, p_key, p_value)
    ON CONFLICT (user_id, category, key)
    DO UPDATE SET 
        value = p_value,
        updated_at = CURRENT_TIMESTAMP;
END;
$$ LANGUAGE plpgsql;

-- カテゴリ別設定取得
CREATE OR REPLACE FUNCTION get_user_preferences_by_category(
    p_user_id UUID,
    p_category VARCHAR(100)
)
RETURNS TABLE(
    key VARCHAR(100),
    value JSONB,
    updated_at TIMESTAMP WITH TIME ZONE
) AS $$
BEGIN
    RETURN QUERY
    SELECT up.key, up.value, up.updated_at
    FROM user_preferences up
    WHERE up.user_id = p_user_id
        AND up.category = p_category
    ORDER BY up.key;
END;
$$ LANGUAGE plpgsql;

-- 設定削除
CREATE OR REPLACE FUNCTION delete_user_preference(
    p_user_id UUID,
    p_category VARCHAR(100),
    p_key VARCHAR(100)
)
RETURNS BOOLEAN AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM user_preferences
    WHERE user_id = p_user_id
        AND category = p_category
        AND key = p_key;
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count > 0;
END;
$$ LANGUAGE plpgsql;

-- 監査ログ統計関数
CREATE OR REPLACE FUNCTION get_audit_stats(
    p_tenant_id UUID DEFAULT NULL,
    p_start_date TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_DATE - INTERVAL '30 days',
    p_end_date TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
)
RETURNS TABLE(
    action VARCHAR(100),
    count BIGINT,
    unique_users BIGINT,
    last_occurrence TIMESTAMP WITH TIME ZONE
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        a.action,
        COUNT(*)::BIGINT as count,
        COUNT(DISTINCT a.user_id)::BIGINT as unique_users,
        MAX(a.created_at) as last_occurrence
    FROM audit_logs a
    WHERE 
        (p_tenant_id IS NULL OR a.tenant_id = p_tenant_id)
        AND a.created_at >= p_start_date
        AND a.created_at <= p_end_date
    GROUP BY a.action
    ORDER BY count DESC;
END;
$$ LANGUAGE plpgsql;

-- 古いパーティション削除関数
CREATE OR REPLACE FUNCTION cleanup_old_audit_partitions(months_to_keep INTEGER DEFAULT 12)
RETURNS INTEGER AS $$
DECLARE
    partition_record RECORD;
    cutoff_date DATE;
    deleted_count INTEGER := 0;
BEGIN
    cutoff_date := date_trunc('month', CURRENT_DATE - (months_to_keep || ' months')::INTERVAL);
    
    FOR partition_record IN
        SELECT schemaname, tablename
        FROM pg_tables
        WHERE tablename LIKE 'audit_logs_y%'
            AND schemaname = 'public'
    LOOP
        -- パーティション名から日付を抽出して比較
        IF (substring(partition_record.tablename from 'y(\d{4})m(\d{2})')) IS NOT NULL THEN
            DECLARE
                year_month TEXT;
                partition_date DATE;
            BEGIN
                year_month := substring(partition_record.tablename from 'y(\d{4})m(\d{2})');
                partition_date := to_date(year_month, 'YYYYMM');
                
                IF partition_date < cutoff_date THEN
                    EXECUTE format('DROP TABLE IF EXISTS %I', partition_record.tablename);
                    deleted_count := deleted_count + 1;
                    RAISE NOTICE 'Dropped old partition: %', partition_record.tablename;
                END IF;
            END;
        END IF;
    END LOOP;
    
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- 監査ログトリガー関数 (自動ログ記録)
CREATE OR REPLACE FUNCTION auto_audit_trigger()
RETURNS TRIGGER AS $$
DECLARE
    action_name VARCHAR(100);
    resource_type_name VARCHAR(100);
    tenant_uuid UUID;
    user_uuid UUID;
BEGIN
    -- アクション名決定
    CASE TG_OP
        WHEN 'INSERT' THEN action_name := 'CREATE';
        WHEN 'UPDATE' THEN action_name := 'UPDATE';
        WHEN 'DELETE' THEN action_name := 'DELETE';
    END CASE;
    
    -- リソースタイプ決定
    resource_type_name := TG_TABLE_NAME;
    
    -- ユーザー・テナント情報取得
    IF TG_OP = 'DELETE' THEN
        user_uuid := OLD.user_id;
        tenant_uuid := OLD.tenant_id;
    ELSE
        user_uuid := NEW.user_id;
        tenant_uuid := NEW.tenant_id;
    END IF;
    
    -- 監査ログ記録
    PERFORM log_audit_event(
        tenant_uuid,
        user_uuid,
        action_name,
        resource_type_name,
        COALESCE(NEW.id::TEXT, OLD.id::TEXT),
        '{}'::JSONB
    );
    
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

-- コメント追加
COMMENT ON TABLE audit_logs IS '監査ログテーブル - 月別パーティション対応';
COMMENT ON COLUMN audit_logs.action IS '実行されたアクション (CREATE, UPDATE, DELETE等)';
COMMENT ON COLUMN audit_logs.resource_type IS 'リソースタイプ (テーブル名等)';
COMMENT ON COLUMN audit_logs.details IS '詳細情報 (JSON)';

COMMENT ON TABLE user_preferences IS 'ユーザー設定詳細テーブル - カテゴリ別管理';
COMMENT ON COLUMN user_preferences.category IS '設定カテゴリ (theme, notifications等)';
COMMENT ON COLUMN user_preferences.key IS '設定キー';
COMMENT ON COLUMN user_preferences.value IS '設定値 (JSON)';