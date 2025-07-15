-- ==========================================
-- 03_sessions.sql
-- セッション管理テーブル作成
-- ==========================================

-- セッション管理テーブル
CREATE TABLE IF NOT EXISTS user_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kratos_session_id VARCHAR(255) UNIQUE NOT NULL,
    device_info JSONB DEFAULT '{}',
    ip_address INET,
    user_agent TEXT,
    active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    last_activity_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- インデックス
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_kratos_session_id ON user_sessions(kratos_session_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_active ON user_sessions(active);
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_user_sessions_last_activity ON user_sessions(last_activity_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_active ON user_sessions(user_id, active) WHERE active = true;

-- updated_at の自動更新トリガー
DROP TRIGGER IF EXISTS update_user_sessions_updated_at ON user_sessions;
CREATE TRIGGER update_user_sessions_updated_at
    BEFORE UPDATE ON user_sessions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- CSRF トークン管理テーブル
CREATE TABLE IF NOT EXISTS csrf_tokens (
    token VARCHAR(255) PRIMARY KEY,
    session_id VARCHAR(255) NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL
);

-- インデックス
CREATE INDEX IF NOT EXISTS idx_csrf_tokens_session_id ON csrf_tokens(session_id);
CREATE INDEX IF NOT EXISTS idx_csrf_tokens_user_id ON csrf_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_csrf_tokens_expires_at ON csrf_tokens(expires_at);

-- デバイス情報バリデーション関数
CREATE OR REPLACE FUNCTION validate_device_info(device_info JSONB)
RETURNS BOOLEAN AS $$
BEGIN
    -- 空のJSONは許可
    IF device_info = '{}'::JSONB THEN
        RETURN TRUE;
    END IF;
    
    -- 基本構造チェック
    IF NOT (jsonb_typeof(device_info) = 'object') THEN
        RETURN FALSE;
    END IF;
    
    -- type の検証（存在する場合）
    IF device_info ? 'type' THEN
        IF NOT (device_info->>'type' IN ('desktop', 'mobile', 'tablet', 'unknown')) THEN
            RETURN FALSE;
        END IF;
    END IF;
    
    RETURN TRUE;
END;
$$ LANGUAGE plpgsql;

-- デバイス情報バリデーション制約
ALTER TABLE user_sessions 
ADD CONSTRAINT valid_device_info 
CHECK (validate_device_info(device_info));

-- セッション関連のヘルパー関数

-- アクティブセッション取得
CREATE OR REPLACE FUNCTION get_active_sessions(user_uuid UUID)
RETURNS SETOF user_sessions AS $$
BEGIN
    RETURN QUERY
    SELECT *
    FROM user_sessions
    WHERE user_id = user_uuid 
        AND active = true 
        AND expires_at > CURRENT_TIMESTAMP
    ORDER BY last_activity_at DESC;
END;
$$ LANGUAGE plpgsql;

-- セッション有効性チェック
CREATE OR REPLACE FUNCTION is_session_valid(kratos_session VARCHAR(255))
RETURNS BOOLEAN AS $$
DECLARE
    session_record user_sessions;
BEGIN
    SELECT * INTO session_record
    FROM user_sessions
    WHERE kratos_session_id = kratos_session
        AND active = true
        AND expires_at > CURRENT_TIMESTAMP;
    
    RETURN FOUND;
END;
$$ LANGUAGE plpgsql;

-- セッション非アクティブ化
CREATE OR REPLACE FUNCTION deactivate_session(kratos_session VARCHAR(255))
RETURNS VOID AS $$
BEGIN
    UPDATE user_sessions
    SET active = false,
        updated_at = CURRENT_TIMESTAMP
    WHERE kratos_session_id = kratos_session;
END;
$$ LANGUAGE plpgsql;

-- 期限切れセッション削除
CREATE OR REPLACE FUNCTION cleanup_expired_sessions()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM user_sessions
    WHERE expires_at < CURRENT_TIMESTAMP - INTERVAL '7 days';
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    
    -- 期限切れCSRFトークンも削除
    DELETE FROM csrf_tokens
    WHERE expires_at < CURRENT_TIMESTAMP;
    
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- セッション活動時刻更新
CREATE OR REPLACE FUNCTION update_session_activity(kratos_session VARCHAR(255))
RETURNS VOID AS $$
BEGIN
    UPDATE user_sessions
    SET last_activity_at = CURRENT_TIMESTAMP,
        updated_at = CURRENT_TIMESTAMP
    WHERE kratos_session_id = kratos_session
        AND active = true;
END;
$$ LANGUAGE plpgsql;

-- CSRF トークン関連関数

-- CSRFトークン作成
CREATE OR REPLACE FUNCTION create_csrf_token(
    token_value VARCHAR(255),
    session_id_value VARCHAR(255),
    user_uuid UUID,
    expires_interval INTERVAL DEFAULT '1 hour'
)
RETURNS VOID AS $$
BEGIN
    INSERT INTO csrf_tokens (token, session_id, user_id, expires_at)
    VALUES (
        token_value,
        session_id_value,
        user_uuid,
        CURRENT_TIMESTAMP + expires_interval
    );
END;
$$ LANGUAGE plpgsql;

-- CSRFトークン検証
CREATE OR REPLACE FUNCTION validate_csrf_token(
    token_value VARCHAR(255),
    session_id_value VARCHAR(255)
)
RETURNS BOOLEAN AS $$
DECLARE
    token_record csrf_tokens;
BEGIN
    SELECT * INTO token_record
    FROM csrf_tokens
    WHERE token = token_value
        AND session_id = session_id_value
        AND expires_at > CURRENT_TIMESTAMP;
    
    RETURN FOUND;
END;
$$ LANGUAGE plpgsql;

-- CSRFトークン削除
CREATE OR REPLACE FUNCTION delete_csrf_token(token_value VARCHAR(255))
RETURNS VOID AS $$
BEGIN
    DELETE FROM csrf_tokens
    WHERE token = token_value;
END;
$$ LANGUAGE plpgsql;

-- 期限切れCSRFトークン自動削除
CREATE OR REPLACE FUNCTION cleanup_expired_csrf_tokens()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM csrf_tokens
    WHERE expires_at < CURRENT_TIMESTAMP;
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- ユーザー削除時のセッション自動削除トリガー
CREATE OR REPLACE FUNCTION cleanup_user_sessions()
RETURNS TRIGGER AS $$
BEGIN
    -- ユーザーのすべてのセッションを非アクティブ化
    UPDATE user_sessions
    SET active = false,
        updated_at = CURRENT_TIMESTAMP
    WHERE user_id = OLD.id;
    
    -- ユーザーのCSRFトークンを削除
    DELETE FROM csrf_tokens
    WHERE user_id = OLD.id;
    
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS cleanup_user_sessions_trigger ON users;
CREATE TRIGGER cleanup_user_sessions_trigger
    BEFORE DELETE ON users
    FOR EACH ROW
    EXECUTE FUNCTION cleanup_user_sessions();

-- セッション統計関数
CREATE OR REPLACE FUNCTION get_session_stats(user_uuid UUID DEFAULT NULL)
RETURNS TABLE(
    total_sessions BIGINT,
    active_sessions BIGINT,
    expired_sessions BIGINT,
    unique_devices BIGINT
) AS $$
BEGIN
    IF user_uuid IS NULL THEN
        -- 全ユーザーの統計
        RETURN QUERY
        SELECT 
            COUNT(*)::BIGINT as total_sessions,
            COUNT(*) FILTER (WHERE active = true AND expires_at > CURRENT_TIMESTAMP)::BIGINT as active_sessions,
            COUNT(*) FILTER (WHERE expires_at <= CURRENT_TIMESTAMP)::BIGINT as expired_sessions,
            COUNT(DISTINCT device_info->>'type')::BIGINT as unique_devices
        FROM user_sessions;
    ELSE
        -- 特定ユーザーの統計
        RETURN QUERY
        SELECT 
            COUNT(*)::BIGINT as total_sessions,
            COUNT(*) FILTER (WHERE active = true AND expires_at > CURRENT_TIMESTAMP)::BIGINT as active_sessions,
            COUNT(*) FILTER (WHERE expires_at <= CURRENT_TIMESTAMP)::BIGINT as expired_sessions,
            COUNT(DISTINCT device_info->>'type')::BIGINT as unique_devices
        FROM user_sessions
        WHERE user_id = user_uuid;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- コメント追加
COMMENT ON TABLE user_sessions IS 'ユーザーセッション管理テーブル - Kratosセッションとの連携';
COMMENT ON COLUMN user_sessions.kratos_session_id IS 'Ory Kratosのセッション ID';
COMMENT ON COLUMN user_sessions.device_info IS 'デバイス情報 (JSON)';
COMMENT ON COLUMN user_sessions.last_activity_at IS '最終活動時刻';

COMMENT ON TABLE csrf_tokens IS 'CSRF トークン管理テーブル';
COMMENT ON COLUMN csrf_tokens.session_id IS 'セッション ID (Kratos)';
COMMENT ON COLUMN csrf_tokens.expires_at IS 'トークン有効期限';