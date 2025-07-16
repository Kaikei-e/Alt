-- Create user_sessions table for session management
CREATE TABLE IF NOT EXISTS user_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kratos_session_id VARCHAR(255) NOT NULL UNIQUE,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_activity_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ip_address INET,
    user_agent TEXT,
    device_info JSONB DEFAULT '{}',
    session_metadata JSONB DEFAULT '{}'
);

-- Create indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_kratos_session_id ON user_sessions(kratos_session_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_active ON user_sessions(active);
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_user_sessions_last_activity_at ON user_sessions(last_activity_at);
CREATE INDEX IF NOT EXISTS idx_user_sessions_created_at ON user_sessions(created_at);

-- Create composite indexes for common queries
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_active ON user_sessions(user_id, active);
CREATE INDEX IF NOT EXISTS idx_user_sessions_active_expires ON user_sessions(active, expires_at);

-- Create trigger to automatically update updated_at
CREATE TRIGGER update_user_sessions_updated_at 
    BEFORE UPDATE ON user_sessions 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Create function to clean up expired sessions
CREATE OR REPLACE FUNCTION cleanup_expired_sessions()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM user_sessions 
    WHERE expires_at < CURRENT_TIMESTAMP - INTERVAL '7 days';
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Create function to get active session count per user
CREATE OR REPLACE FUNCTION get_active_session_count(p_user_id UUID)
RETURNS INTEGER AS $$
DECLARE
    session_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO session_count
    FROM user_sessions 
    WHERE user_id = p_user_id 
    AND active = true 
    AND expires_at > CURRENT_TIMESTAMP;
    
    RETURN session_count;
END;
$$ LANGUAGE plpgsql;