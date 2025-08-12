-- Migration: Add tenant_id columns to existing tables for multi-tenant support
-- Created: 2025-08-12 00:29:00
-- Atlas Version: v0.35

-- Add tenant_id column to feeds table
ALTER TABLE feeds ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;

-- Add tenant_id column to articles table  
ALTER TABLE articles ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;

-- Add tenant_id column to read_status table
ALTER TABLE read_status ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;

-- Add tenant_id column to favorite_feeds table (if exists)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'favorite_feeds') THEN
        ALTER TABLE favorite_feeds ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;
    END IF;
END
$$;

-- Add tenant_id column to user_feeds table (if exists)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'user_feeds') THEN
        ALTER TABLE user_feeds ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;
    END IF;
END
$$;

-- Create indexes for tenant_id columns for performance
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_feeds_tenant_id ON feeds(tenant_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_articles_tenant_id ON articles(tenant_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_read_status_tenant_id ON read_status(tenant_id);

-- Create composite indexes for common queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_feeds_tenant_active ON feeds(tenant_id, is_active) WHERE is_active = true;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_articles_tenant_published ON articles(tenant_id, published_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_read_status_tenant_user ON read_status(tenant_id, user_id);

-- Update existing data to assign a default tenant (if needed)
-- This assumes a default tenant exists - this should be handled carefully in production
DO $$
DECLARE 
    default_tenant_id UUID;
BEGIN
    -- Find or create a default tenant
    SELECT id INTO default_tenant_id FROM tenants WHERE slug = 'default' LIMIT 1;
    
    IF default_tenant_id IS NULL THEN
        INSERT INTO tenants (id, name, slug, description, status, subscription_tier, max_users, max_feeds, settings, created_at, updated_at)
        VALUES (
            gen_random_uuid(),
            'Default Tenant',
            'default',
            'Default tenant for existing data',
            'active',
            'free',
            5,
            50,
            '{"theme": "light", "language": "en"}',
            NOW(),
            NOW()
        ) RETURNING id INTO default_tenant_id;
    END IF;

    -- Update feeds table
    UPDATE feeds SET tenant_id = default_tenant_id WHERE tenant_id IS NULL;
    
    -- Update articles table
    UPDATE articles SET tenant_id = default_tenant_id WHERE tenant_id IS NULL;
    
    -- Update read_status table
    UPDATE read_status SET tenant_id = default_tenant_id WHERE tenant_id IS NULL;
    
    -- Update favorite_feeds table if exists
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'favorite_feeds') THEN
        UPDATE favorite_feeds SET tenant_id = default_tenant_id WHERE tenant_id IS NULL;
    END IF;
    
    -- Update user_feeds table if exists
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'user_feeds') THEN
        UPDATE user_feeds SET tenant_id = default_tenant_id WHERE tenant_id IS NULL;
    END IF;
END
$$;

-- Make tenant_id columns NOT NULL after populating with default values
ALTER TABLE feeds ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE articles ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE read_status ALTER COLUMN tenant_id SET NOT NULL;

-- Add NOT NULL constraints to other tables if they exist
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'favorite_feeds') THEN
        ALTER TABLE favorite_feeds ALTER COLUMN tenant_id SET NOT NULL;
    END IF;
    
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'user_feeds') THEN
        ALTER TABLE user_feeds ALTER COLUMN tenant_id SET NOT NULL;
    END IF;
END
$$;