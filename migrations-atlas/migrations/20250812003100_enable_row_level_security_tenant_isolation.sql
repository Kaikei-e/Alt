-- Migration: Enable Row Level Security for Tenant Isolation
-- Created: 2025-08-12 03:31:00
-- Atlas Version: v0.35

-- Add tenant_id columns to main tables if not already present
ALTER TABLE feeds ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE articles ADD COLUMN IF NOT EXISTS tenant_id UUID;

-- Create tenant_id indexes for better performance
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_feeds_tenant_id ON feeds(tenant_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_articles_tenant_id ON articles(tenant_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_read_status_user_tenant ON read_status(user_id, created_at) WHERE user_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_favorite_feeds_user_tenant ON favorite_feeds(user_id, created_at) WHERE user_id IS NOT NULL;

-- Enable Row Level Security on all tenant-isolated tables
ALTER TABLE feeds ENABLE ROW LEVEL SECURITY;
ALTER TABLE articles ENABLE ROW LEVEL SECURITY;
ALTER TABLE read_status ENABLE ROW LEVEL SECURITY;
ALTER TABLE favorite_feeds ENABLE ROW LEVEL SECURITY;

-- Create tenant_id setting function for session-level isolation
CREATE OR REPLACE FUNCTION set_current_tenant(tenant_uuid uuid)
RETURNS void AS $$
BEGIN
    PERFORM set_config('app.current_tenant_id', tenant_uuid::text, true);
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Create helper function to get current tenant ID
CREATE OR REPLACE FUNCTION get_current_tenant_id()
RETURNS uuid AS $$
DECLARE
    tenant_uuid uuid;
BEGIN
    SELECT current_setting('app.current_tenant_id', true)::uuid INTO tenant_uuid;
    RETURN tenant_uuid;
EXCEPTION
    WHEN OTHERS THEN
        RETURN NULL;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Row Level Security Policies for tenant isolation

-- Feeds table: Allow access only to feeds belonging to current tenant
CREATE POLICY tenant_isolation_feeds ON feeds
    FOR ALL
    TO authenticated_users
    USING (
        tenant_id = get_current_tenant_id() 
        OR get_current_tenant_id() IS NULL  -- Allow unrestricted access when tenant not set
    )
    WITH CHECK (
        tenant_id = get_current_tenant_id()
        AND get_current_tenant_id() IS NOT NULL
    );

-- Articles table: Allow access only to articles belonging to current tenant
CREATE POLICY tenant_isolation_articles ON articles
    FOR ALL 
    TO authenticated_users
    USING (
        tenant_id = get_current_tenant_id()
        OR get_current_tenant_id() IS NULL  -- Allow unrestricted access when tenant not set
    )
    WITH CHECK (
        tenant_id = get_current_tenant_id()
        AND get_current_tenant_id() IS NOT NULL
    );

-- Read status table: Restrict access to users within current tenant
CREATE POLICY tenant_isolation_read_status ON read_status
    FOR ALL
    TO authenticated_users
    USING (
        get_current_tenant_id() IS NULL  -- Allow unrestricted access when tenant not set
        OR user_id IN (
            SELECT id FROM auth_service.users 
            WHERE tenant_id = get_current_tenant_id()
        )
    )
    WITH CHECK (
        get_current_tenant_id() IS NOT NULL
        AND user_id IN (
            SELECT id FROM auth_service.users
            WHERE tenant_id = get_current_tenant_id()
        )
    );

-- Favorite feeds table: Restrict access to users within current tenant
CREATE POLICY tenant_isolation_favorite_feeds ON favorite_feeds
    FOR ALL
    TO authenticated_users  
    USING (
        get_current_tenant_id() IS NULL  -- Allow unrestricted access when tenant not set
        OR user_id IN (
            SELECT id FROM auth_service.users
            WHERE tenant_id = get_current_tenant_id()
        )
    )
    WITH CHECK (
        get_current_tenant_id() IS NOT NULL
        AND user_id IN (
            SELECT id FROM auth_service.users
            WHERE tenant_id = get_current_tenant_id()
        )
    );

-- Grant execute permissions on tenant functions to application users
GRANT EXECUTE ON FUNCTION set_current_tenant(uuid) TO authenticated_users;
GRANT EXECUTE ON FUNCTION get_current_tenant_id() TO authenticated_users;

-- Create trigger function to automatically set tenant_id on insert if not provided
CREATE OR REPLACE FUNCTION set_tenant_id_on_insert()
RETURNS TRIGGER AS $$
BEGIN
    -- Only set tenant_id if it's not already provided and current tenant is set
    IF NEW.tenant_id IS NULL AND get_current_tenant_id() IS NOT NULL THEN
        NEW.tenant_id := get_current_tenant_id();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create triggers for automatic tenant_id assignment
CREATE TRIGGER trigger_feeds_set_tenant_id
    BEFORE INSERT ON feeds
    FOR EACH ROW
    EXECUTE FUNCTION set_tenant_id_on_insert();

CREATE TRIGGER trigger_articles_set_tenant_id
    BEFORE INSERT ON articles
    FOR EACH ROW
    EXECUTE FUNCTION set_tenant_id_on_insert();

-- Comment on policies for documentation
COMMENT ON POLICY tenant_isolation_feeds ON feeds IS 'Enforces tenant isolation by restricting access to feeds belonging to the current session tenant';
COMMENT ON POLICY tenant_isolation_articles ON articles IS 'Enforces tenant isolation by restricting access to articles belonging to the current session tenant';
COMMENT ON POLICY tenant_isolation_read_status ON read_status IS 'Enforces tenant isolation by restricting access to read status records of users within the current tenant';
COMMENT ON POLICY tenant_isolation_favorite_feeds ON favorite_feeds IS 'Enforces tenant isolation by restricting access to favorite feeds of users within the current tenant';