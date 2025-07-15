-- Drop partition creation function
DROP FUNCTION IF EXISTS create_monthly_audit_partition(TEXT, DATE);

-- Drop user_preferences table
DROP TRIGGER IF EXISTS update_user_preferences_updated_at ON user_preferences;
DROP INDEX IF EXISTS idx_user_preferences_category;
DROP INDEX IF EXISTS idx_user_preferences_user_id;
DROP TABLE IF EXISTS user_preferences;

-- Drop audit logs partitions
DROP TABLE IF EXISTS audit_logs_y2025m02;
DROP TABLE IF EXISTS audit_logs_y2025m01;

-- Drop audit_logs table
DROP INDEX IF EXISTS idx_audit_logs_created_at;
DROP INDEX IF EXISTS idx_audit_logs_action;
DROP INDEX IF EXISTS idx_audit_logs_user_id;
DROP INDEX IF EXISTS idx_audit_logs_tenant_id;
DROP TABLE IF EXISTS audit_logs;