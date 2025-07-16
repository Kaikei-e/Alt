-- Create users table linked to Kratos identities
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    kratos_id UUID NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    role VARCHAR(50) NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin', 'super_admin')),
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleted')),
    preferences JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMP WITH TIME ZONE,
    email_verified BOOLEAN NOT NULL DEFAULT false,
    phone_verified BOOLEAN NOT NULL DEFAULT false,
    two_factor_enabled BOOLEAN NOT NULL DEFAULT false,
    
    -- Ensure unique email per tenant
    UNIQUE(tenant_id, email)
);

-- Create indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_users_kratos_id ON users(kratos_id);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
CREATE INDEX IF NOT EXISTS idx_users_created_at ON users(created_at);
CREATE INDEX IF NOT EXISTS idx_users_last_login_at ON users(last_login_at);
CREATE INDEX IF NOT EXISTS idx_users_tenant_email ON users(tenant_id, email);

-- Create trigger to automatically update updated_at
CREATE TRIGGER update_users_updated_at 
    BEFORE UPDATE ON users 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Create default user for migration
INSERT INTO users (
    id,
    tenant_id,
    kratos_id,
    email,
    name,
    role,
    status,
    preferences,
    email_verified
) VALUES (
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    'admin@default.tenant',
    'Default Admin',
    'admin',
    'active',
    '{"theme": "auto", "language": "en", "notifications": {"email": true, "push": false}}',
    true
) ON CONFLICT (kratos_id) DO NOTHING;