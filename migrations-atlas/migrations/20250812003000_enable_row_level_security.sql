-- Migration: Enable Row Level Security for tenant isolation
-- Created: 2025-08-12 00:30:00
-- Atlas Version: v0.35

-- RLS有効化
ALTER TABLE feeds ENABLE ROW LEVEL SECURITY;
ALTER TABLE articles ENABLE ROW LEVEL SECURITY;
ALTER TABLE read_status ENABLE ROW LEVEL SECURITY;
ALTER TABLE favorite_feeds ENABLE ROW LEVEL SECURITY;

-- テナント分離ポリシー
CREATE POLICY tenant_isolation_feeds ON feeds
    FOR ALL
    TO authenticated_users
    USING (tenant_id = current_setting('app.current_tenant_id')::uuid);

CREATE POLICY tenant_isolation_articles ON articles
    FOR ALL 
    TO authenticated_users
    USING (tenant_id = current_setting('app.current_tenant_id')::uuid);

CREATE POLICY tenant_isolation_read_status ON read_status
    FOR ALL
    TO authenticated_users
    USING (
        user_id IN (
            SELECT id FROM auth_service.users 
            WHERE tenant_id = current_setting('app.current_tenant_id')::uuid
        )
    );

CREATE POLICY tenant_isolation_favorite_feeds ON favorite_feeds
    FOR ALL
    TO authenticated_users  
    USING (
        user_id IN (
            SELECT id FROM auth_service.users
            WHERE tenant_id = current_setting('app.current_tenant_id')::uuid
        )
    );

-- テナントID設定関数
CREATE OR REPLACE FUNCTION set_current_tenant(tenant_uuid uuid)
RETURNS void AS $$
BEGIN
    PERFORM set_config('app.current_tenant_id', tenant_uuid::text, true);
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- authenticated_users ロール作成（存在しない場合）
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'authenticated_users') THEN
        CREATE ROLE authenticated_users;
    END IF;
END
$$;

-- 既存のアプリケーションロールに authenticated_users を付与
GRANT authenticated_users TO alt_user;
GRANT authenticated_users TO auth_user;