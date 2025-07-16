-- Auth Service Test Data Initialization
-- This script inserts test data for integration testing

-- Insert test tenants
INSERT INTO tenants (id, name, domain, settings) VALUES
(
    '550e8400-e29b-41d4-a716-446655440000',
    'Test Tenant 1',
    'test1.example.com',
    '{"max_users": 100, "features": ["auth", "audit"]}'
),
(
    '550e8400-e29b-41d4-a716-446655440001',
    'Test Tenant 2',
    'test2.example.com',
    '{"max_users": 50, "features": ["auth"]}'
) ON CONFLICT (id) DO NOTHING;

-- Insert test users
INSERT INTO users (id, tenant_id, kratos_id, email, name, role, status, email_verified) VALUES
(
    '660e8400-e29b-41d4-a716-446655440000',
    '550e8400-e29b-41d4-a716-446655440000',
    '770e8400-e29b-41d4-a716-446655440000',
    'testuser1@example.com',
    'Test User 1',
    'user',
    'active',
    true
),
(
    '660e8400-e29b-41d4-a716-446655440001',
    '550e8400-e29b-41d4-a716-446655440000',
    '770e8400-e29b-41d4-a716-446655440001',
    'testadmin@example.com',
    'Test Admin',
    'admin',
    'active',
    true
),
(
    '660e8400-e29b-41d4-a716-446655440002',
    '550e8400-e29b-41d4-a716-446655440001',
    '770e8400-e29b-41d4-a716-446655440002',
    'testuser2@example.com',
    'Test User 2',
    'user',
    'active',
    true
),
(
    '660e8400-e29b-41d4-a716-446655440003',
    '550e8400-e29b-41d4-a716-446655440000',
    '770e8400-e29b-41d4-a716-446655440003',
    'inactive@example.com',
    'Inactive User',
    'user',
    'inactive',
    false
) ON CONFLICT (id) DO NOTHING;

-- Insert test user sessions
INSERT INTO user_sessions (id, user_id, kratos_session_id, active, expires_at, ip_address, user_agent) VALUES
(
    '880e8400-e29b-41d4-a716-446655440000',
    '660e8400-e29b-41d4-a716-446655440000',
    'test-kratos-session-1',
    true,
    CURRENT_TIMESTAMP + INTERVAL '1 hour',
    '192.168.1.100',
    'Mozilla/5.0 (Test Browser)'
),
(
    '880e8400-e29b-41d4-a716-446655440001',
    '660e8400-e29b-41d4-a716-446655440001',
    'test-kratos-session-2',
    true,
    CURRENT_TIMESTAMP + INTERVAL '2 hours',
    '192.168.1.101',
    'Mozilla/5.0 (Test Browser Admin)'
),
(
    '880e8400-e29b-41d4-a716-446655440002',
    '660e8400-e29b-41d4-a716-446655440000',
    'test-kratos-session-expired',
    false,
    CURRENT_TIMESTAMP - INTERVAL '1 hour',
    '192.168.1.100',
    'Mozilla/5.0 (Test Browser)'
) ON CONFLICT (id) DO NOTHING;

-- Insert test CSRF tokens
INSERT INTO csrf_tokens (id, token, session_id, user_id, expires_at, used) VALUES
(
    '990e8400-e29b-41d4-a716-446655440000',
    'test-csrf-token-1234567890abcdef1234567890abcdef',
    'test-kratos-session-1',
    '660e8400-e29b-41d4-a716-446655440000',
    CURRENT_TIMESTAMP + INTERVAL '30 minutes',
    false
),
(
    '990e8400-e29b-41d4-a716-446655440001',
    'test-csrf-token-used-1234567890abcdef1234567890ab',
    'test-kratos-session-1',
    '660e8400-e29b-41d4-a716-446655440000',
    CURRENT_TIMESTAMP + INTERVAL '30 minutes',
    true
),
(
    '990e8400-e29b-41d4-a716-446655440002',
    'test-csrf-token-expired-1234567890abcdef1234567890',
    'test-kratos-session-1',
    '660e8400-e29b-41d4-a716-446655440000',
    CURRENT_TIMESTAMP - INTERVAL '10 minutes',
    false
) ON CONFLICT (id) DO NOTHING;

-- Insert test user preferences
INSERT INTO user_preferences (id, user_id, theme, language, notifications, feed_settings) VALUES
(
    'aa0e8400-e29b-41d4-a716-446655440000',
    '660e8400-e29b-41d4-a716-446655440000',
    'dark',
    'en',
    '{"email": true, "push": true}',
    '{"auto_mark_read": false, "summary_length": "long"}'
),
(
    'aa0e8400-e29b-41d4-a716-446655440001',
    '660e8400-e29b-41d4-a716-446655440001',
    'light',
    'ja',
    '{"email": false, "push": false}',
    '{"auto_mark_read": true, "summary_length": "short"}'
) ON CONFLICT (id) DO NOTHING;

-- Insert test audit logs
INSERT INTO audit_logs (id, tenant_id, user_id, action, resource_type, resource_id, details, ip_address) VALUES
(
    'bb0e8400-e29b-41d4-a716-446655440000',
    '550e8400-e29b-41d4-a716-446655440000',
    '660e8400-e29b-41d4-a716-446655440000',
    'LOGIN',
    'session',
    'test-kratos-session-1',
    '{"method": "password", "success": true}',
    '192.168.1.100'
),
(
    'bb0e8400-e29b-41d4-a716-446655440001',
    '550e8400-e29b-41d4-a716-446655440000',
    '660e8400-e29b-41d4-a716-446655440001',
    'LOGIN',
    'session',
    'test-kratos-session-2',
    '{"method": "password", "success": true}',
    '192.168.1.101'
),
(
    'bb0e8400-e29b-41d4-a716-446655440002',
    '550e8400-e29b-41d4-a716-446655440000',
    '660e8400-e29b-41d4-a716-446655440000',
    'LOGOUT',
    'session',
    'test-kratos-session-expired',
    '{"reason": "expired"}',
    '192.168.1.100'
) ON CONFLICT (id) DO NOTHING;