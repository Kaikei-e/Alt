-- Create csrf_tokens table for CSRF protection
CREATE TABLE IF NOT EXISTS csrf_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token VARCHAR(255) NOT NULL UNIQUE,
    session_id VARCHAR(255) NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    used BOOLEAN NOT NULL DEFAULT false,
    used_at TIMESTAMP WITH TIME ZONE,
    ip_address INET,
    user_agent TEXT
);

-- Create indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_csrf_tokens_token ON csrf_tokens(token);
CREATE INDEX IF NOT EXISTS idx_csrf_tokens_session_id ON csrf_tokens(session_id);
CREATE INDEX IF NOT EXISTS idx_csrf_tokens_user_id ON csrf_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_csrf_tokens_expires_at ON csrf_tokens(expires_at);
CREATE INDEX IF NOT EXISTS idx_csrf_tokens_created_at ON csrf_tokens(created_at);
CREATE INDEX IF NOT EXISTS idx_csrf_tokens_used ON csrf_tokens(used);

-- Create composite indexes for common queries
CREATE INDEX IF NOT EXISTS idx_csrf_tokens_token_used ON csrf_tokens(token, used);
CREATE INDEX IF NOT EXISTS idx_csrf_tokens_session_used ON csrf_tokens(session_id, used);
CREATE INDEX IF NOT EXISTS idx_csrf_tokens_user_expires ON csrf_tokens(user_id, expires_at);

-- Create function to clean up expired CSRF tokens
CREATE OR REPLACE FUNCTION cleanup_expired_csrf_tokens()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM csrf_tokens 
    WHERE expires_at < CURRENT_TIMESTAMP - INTERVAL '1 day';
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Create function to validate CSRF token
CREATE OR REPLACE FUNCTION validate_csrf_token(p_token VARCHAR(255), p_session_id VARCHAR(255))
RETURNS BOOLEAN AS $$
DECLARE
    token_valid BOOLEAN := false;
BEGIN
    SELECT true INTO token_valid
    FROM csrf_tokens 
    WHERE token = p_token 
    AND session_id = p_session_id
    AND used = false 
    AND expires_at > CURRENT_TIMESTAMP;
    
    -- Mark token as used
    UPDATE csrf_tokens 
    SET used = true, used_at = CURRENT_TIMESTAMP
    WHERE token = p_token 
    AND session_id = p_session_id
    AND used = false;
    
    RETURN COALESCE(token_valid, false);
END;
$$ LANGUAGE plpgsql;