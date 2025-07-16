-- Drop user_sessions table and related objects
DROP FUNCTION IF EXISTS get_active_session_count(UUID);
DROP FUNCTION IF EXISTS cleanup_expired_sessions();
DROP TRIGGER IF EXISTS update_user_sessions_updated_at ON user_sessions;
DROP TABLE IF EXISTS user_sessions CASCADE;