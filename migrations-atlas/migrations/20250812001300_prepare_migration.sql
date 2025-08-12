-- Migration: prepare for data migration
-- File: migrations-atlas/migrations/20250812001300_prepare_migration.sql
-- Author: Claude Code
-- Date: 2025-08-12
-- Purpose: Create default legacy user in Kratos database for existing data compatibility

-- Step 1: Create backup tables (already exist, but ensure they're backed up)
-- Note: This will be done via application logic as needed

-- Step 2: Ensure default user exists in Kratos identities table
-- Insert legacy user identity into Kratos
INSERT INTO identities (
    id,
    schema_id,
    traits,
    state,
    created_at,
    updated_at,
    nid
) VALUES (
    '00000000-0000-0000-0000-000000000001',
    'default',
    '{"email": "legacy@alt-reader.local", "name": {"first": "Legacy", "last": "User"}, "tenant_id": "00000000-0000-0000-0000-000000000001", "preferences": {"migrated": true, "theme": "auto", "language": "en", "notifications": {"email": true, "push": false}}}',
    'active',
    NOW(),
    NOW(),
    (SELECT id FROM networks LIMIT 1)
) ON CONFLICT (id) DO UPDATE SET
    traits = EXCLUDED.traits,
    updated_at = NOW();

-- Step 3: Create identity credentials for the legacy user (password-based)
-- Note: This creates a basic password credential entry
-- The actual password hash would be set via Kratos API for security
INSERT INTO identity_credentials (
    id,
    config,
    identity_credential_type_id,
    identity_id,
    created_at,
    updated_at,
    nid
) VALUES (
    gen_random_uuid(),
    '{"hashed_password": "$2a$12$legacy.user.placeholder.hash"}',
    (SELECT id FROM identity_credential_types WHERE name = 'password' LIMIT 1),
    '00000000-0000-0000-0000-000000000001',
    NOW(),
    NOW(),
    (SELECT id FROM networks LIMIT 1)
) ON CONFLICT DO NOTHING;

-- Step 4: Create identity credential identifier
INSERT INTO identity_credential_identifiers (
    id,
    identifier,
    identity_credential_id,
    created_at,
    updated_at,
    nid
) VALUES (
    gen_random_uuid(),
    'legacy@alt-reader.local',
    (SELECT id FROM identity_credentials WHERE identity_id = '00000000-0000-0000-0000-000000000001' LIMIT 1),
    NOW(),
    NOW(),
    (SELECT id FROM networks LIMIT 1)
) ON CONFLICT DO NOTHING;

-- Step 5: Create verifiable address for email
INSERT INTO identity_verifiable_addresses (
    id,
    status,
    via,
    verified,
    value,
    verified_at,
    created_at,
    updated_at,
    identity_id,
    nid
) VALUES (
    gen_random_uuid(),
    'completed',
    'email',
    true,
    'legacy@alt-reader.local',
    NOW(),
    NOW(),
    NOW(),
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM networks LIMIT 1)
) ON CONFLICT DO NOTHING;