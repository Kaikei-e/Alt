-- Drop cleanup function
DROP FUNCTION IF EXISTS cleanup_expired_sessions_and_tokens();

-- Drop CSRF tokens table
DROP INDEX IF EXISTS idx_csrf_tokens_expires_at;
DROP INDEX IF EXISTS idx_csrf_tokens_user_id;
DROP INDEX IF EXISTS idx_csrf_tokens_session_id;
DROP TABLE IF EXISTS csrf_tokens;

-- Drop user_sessions table
DROP TRIGGER IF EXISTS update_user_sessions_updated_at ON user_sessions;
DROP INDEX IF EXISTS idx_user_sessions_expires_at;
DROP INDEX IF EXISTS idx_user_sessions_active;
DROP INDEX IF EXISTS idx_user_sessions_kratos_session_id;
DROP INDEX IF EXISTS idx_user_sessions_user_id;
DROP TABLE IF EXISTS user_sessions;