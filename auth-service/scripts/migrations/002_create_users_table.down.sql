-- Drop trigger
DROP TRIGGER IF EXISTS update_users_updated_at ON users;

-- Drop indexes
DROP INDEX IF EXISTS idx_users_created_at;
DROP INDEX IF EXISTS idx_users_status;
DROP INDEX IF EXISTS idx_users_email;
DROP INDEX IF EXISTS idx_users_kratos_identity_id;
DROP INDEX IF EXISTS idx_users_tenant_id;

-- Drop users table
DROP TABLE IF EXISTS users;