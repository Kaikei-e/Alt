-- Drop user_preferences table and related objects
DROP FUNCTION IF EXISTS set_user_preference(UUID, VARCHAR(50), VARCHAR(100), JSONB);
DROP FUNCTION IF EXISTS get_user_preferences(UUID, VARCHAR(50));
DROP TRIGGER IF EXISTS update_user_preferences_updated_at ON user_preferences;
DROP TABLE IF EXISTS user_preferences CASCADE;