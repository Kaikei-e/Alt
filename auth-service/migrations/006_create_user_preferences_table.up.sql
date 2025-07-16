-- Create user_preferences table for user settings
CREATE TABLE IF NOT EXISTS user_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category VARCHAR(50) NOT NULL,
    key VARCHAR(100) NOT NULL,
    value JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    -- Ensure unique preference per user
    UNIQUE(user_id, category, key)
);

-- Create indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_user_preferences_user_id ON user_preferences(user_id);
CREATE INDEX IF NOT EXISTS idx_user_preferences_category ON user_preferences(category);
CREATE INDEX IF NOT EXISTS idx_user_preferences_key ON user_preferences(key);
CREATE INDEX IF NOT EXISTS idx_user_preferences_created_at ON user_preferences(created_at);

-- Create composite indexes for common queries
CREATE INDEX IF NOT EXISTS idx_user_preferences_user_category ON user_preferences(user_id, category);
CREATE INDEX IF NOT EXISTS idx_user_preferences_user_key ON user_preferences(user_id, key);

-- Create trigger to automatically update updated_at
CREATE TRIGGER update_user_preferences_updated_at 
    BEFORE UPDATE ON user_preferences 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Create function to get user preferences by category
CREATE OR REPLACE FUNCTION get_user_preferences(p_user_id UUID, p_category VARCHAR(50) DEFAULT NULL)
RETURNS JSONB AS $$
DECLARE
    preferences JSONB := '{}';
    pref_record RECORD;
BEGIN
    FOR pref_record IN 
        SELECT category, key, value
        FROM user_preferences
        WHERE user_id = p_user_id
        AND (p_category IS NULL OR category = p_category)
        ORDER BY category, key
    LOOP
        preferences := jsonb_set(
            preferences, 
            ARRAY[pref_record.category, pref_record.key], 
            pref_record.value
        );
    END LOOP;
    
    RETURN preferences;
END;
$$ LANGUAGE plpgsql;

-- Create function to set user preference
CREATE OR REPLACE FUNCTION set_user_preference(
    p_user_id UUID,
    p_category VARCHAR(50),
    p_key VARCHAR(100),
    p_value JSONB
)
RETURNS VOID AS $$
BEGIN
    INSERT INTO user_preferences (user_id, category, key, value)
    VALUES (p_user_id, p_category, p_key, p_value)
    ON CONFLICT (user_id, category, key) 
    DO UPDATE SET value = p_value, updated_at = CURRENT_TIMESTAMP;
END;
$$ LANGUAGE plpgsql;

-- Insert default preferences for default user
INSERT INTO user_preferences (user_id, category, key, value) VALUES
('00000000-0000-0000-0000-000000000001', 'appearance', 'theme', '"auto"'),
('00000000-0000-0000-0000-000000000001', 'appearance', 'language', '"en"'),
('00000000-0000-0000-0000-000000000001', 'notifications', 'email', 'true'),
('00000000-0000-0000-0000-000000000001', 'notifications', 'push', 'false'),
('00000000-0000-0000-0000-000000000001', 'feeds', 'auto_mark_read', 'true'),
('00000000-0000-0000-0000-000000000001', 'feeds', 'summary_length', '"medium"'),
('00000000-0000-0000-0000-000000000001', 'feeds', 'refresh_interval', '300'),
('00000000-0000-0000-0000-000000000001', 'privacy', 'analytics', 'true'),
('00000000-0000-0000-0000-000000000001', 'privacy', 'personalization', 'true')
ON CONFLICT (user_id, category, key) DO NOTHING;