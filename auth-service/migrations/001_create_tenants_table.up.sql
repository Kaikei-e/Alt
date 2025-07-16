-- Create tenants table for multi-tenant support
CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleted')),
    subscription_tier VARCHAR(50) NOT NULL DEFAULT 'free' CHECK (subscription_tier IN ('free', 'basic', 'premium', 'enterprise')),
    max_users INTEGER NOT NULL DEFAULT 10,
    max_feeds INTEGER NOT NULL DEFAULT 100,
    settings JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_tenants_slug ON tenants(slug);
CREATE INDEX IF NOT EXISTS idx_tenants_status ON tenants(status);
CREATE INDEX IF NOT EXISTS idx_tenants_subscription_tier ON tenants(subscription_tier);
CREATE INDEX IF NOT EXISTS idx_tenants_created_at ON tenants(created_at);

-- Create default tenant for migration
INSERT INTO tenants (
    id, 
    name, 
    slug, 
    description, 
    status, 
    subscription_tier, 
    max_users, 
    max_feeds
) VALUES (
    '00000000-0000-0000-0000-000000000001',
    'Default Tenant',
    'default',
    'Default tenant for existing users during migration',
    'active',
    'enterprise',
    1000,
    10000
) ON CONFLICT (slug) DO NOTHING;

-- Create trigger to automatically update updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_tenants_updated_at 
    BEFORE UPDATE ON tenants 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();