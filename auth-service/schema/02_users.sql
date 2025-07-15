-- ==========================================
-- 02_users.sql
-- ユーザー管理テーブル作成
-- ==========================================

-- ユーザー管理テーブル
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kratos_identity_id UUID UNIQUE NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    role VARCHAR(50) DEFAULT 'user' CHECK (role IN ('admin', 'user', 'readonly')),
    status VARCHAR(20) DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'suspended')),
    preferences JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE NULL,

    -- テナント内でメールアドレスは一意
    UNIQUE(tenant_id, email)
);

-- インデックス
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_users_kratos_identity_id ON users(kratos_identity_id);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
CREATE INDEX IF NOT EXISTS idx_users_created_at ON users(created_at);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_tenant_email ON users(tenant_id, email);
CREATE INDEX IF NOT EXISTS idx_users_last_login ON users(last_login_at DESC) WHERE last_login_at IS NOT NULL;

-- updated_at の自動更新トリガー
DROP TRIGGER IF EXISTS update_users_updated_at ON users;
CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ユーザー設定のJSON検証関数
CREATE OR REPLACE FUNCTION validate_user_preferences(preferences JSONB)
RETURNS BOOLEAN AS $$
BEGIN
    -- 空のJSONは許可
    IF preferences = '{}'::JSONB THEN
        RETURN TRUE;
    END IF;
    
    -- 基本構造チェック
    IF NOT (jsonb_typeof(preferences) = 'object') THEN
        RETURN FALSE;
    END IF;
    
    -- theme の検証（存在する場合）
    IF preferences ? 'theme' THEN
        IF NOT (preferences->>'theme' IN ('light', 'dark', 'auto')) THEN
            RETURN FALSE;
        END IF;
    END IF;
    
    -- language の検証（存在する場合）
    IF preferences ? 'language' THEN
        IF NOT (preferences->>'language' ~ '^[a-z]{2}(-[A-Z]{2})?$') THEN
            RETURN FALSE;
        END IF;
    END IF;
    
    -- notifications の検証（存在する場合）
    IF preferences ? 'notifications' THEN
        IF NOT (jsonb_typeof(preferences->'notifications') = 'object') THEN
            RETURN FALSE;
        END IF;
    END IF;
    
    RETURN TRUE;
END;
$$ LANGUAGE plpgsql;

-- 設定バリデーション制約
ALTER TABLE users 
ADD CONSTRAINT valid_user_preferences 
CHECK (validate_user_preferences(preferences));

-- メール形式バリデーション制約
ALTER TABLE users 
ADD CONSTRAINT valid_email_format 
CHECK (email ~* '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$');

-- デフォルトユーザー作成
INSERT INTO users (id, kratos_identity_id, tenant_id, email, name, role, status, preferences)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    'default@alt.local',
    'Default User',
    'admin',
    'active',
    '{
        "theme": "auto",
        "language": "en",
        "notifications": {"email": true, "push": false},
        "feed_settings": {"auto_mark_read": true, "summary_length": "medium"}
    }'::JSONB
) ON CONFLICT (id) DO NOTHING;

-- ユーザー関連のヘルパー関数
CREATE OR REPLACE FUNCTION get_user_by_kratos_id(kratos_uuid UUID)
RETURNS users AS $$
DECLARE
    user_record users;
BEGIN
    SELECT * INTO user_record 
    FROM users 
    WHERE kratos_identity_id = kratos_uuid AND deleted_at IS NULL;
    
    IF NOT FOUND THEN
        RAISE EXCEPTION 'User with Kratos ID % not found', kratos_uuid;
    END IF;
    
    RETURN user_record;
END;
$$ LANGUAGE plpgsql;

-- ユーザーアクティブ状態チェック関数
CREATE OR REPLACE FUNCTION is_user_active(user_uuid UUID)
RETURNS BOOLEAN AS $$
DECLARE
    user_status VARCHAR(20);
    tenant_active BOOLEAN;
BEGIN
    SELECT u.status, is_tenant_active(u.tenant_id) 
    INTO user_status, tenant_active
    FROM users u
    WHERE u.id = user_uuid AND u.deleted_at IS NULL;
    
    RETURN user_status = 'active' AND tenant_active;
END;
$$ LANGUAGE plpgsql;

-- テナント内のユーザー数カウント関数
CREATE OR REPLACE FUNCTION count_tenant_users(tenant_uuid UUID)
RETURNS INTEGER AS $$
DECLARE
    user_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO user_count
    FROM users
    WHERE tenant_id = tenant_uuid AND deleted_at IS NULL;
    
    RETURN user_count;
END;
$$ LANGUAGE plpgsql;

-- ユーザー作成時のテナント制限チェック
CREATE OR REPLACE FUNCTION check_tenant_user_limit()
RETURNS TRIGGER AS $$
DECLARE
    current_count INTEGER;
    max_users INTEGER;
BEGIN
    -- テナントの最大ユーザー数を取得
    SELECT (settings->'limits'->>'max_users')::INTEGER
    INTO max_users
    FROM tenants
    WHERE id = NEW.tenant_id;
    
    -- 現在のユーザー数を取得
    current_count := count_tenant_users(NEW.tenant_id);
    
    -- 制限チェック（新規作成時のみ）
    IF TG_OP = 'INSERT' AND current_count >= max_users THEN
        RAISE EXCEPTION 'Tenant user limit exceeded. Max: %, Current: %', max_users, current_count;
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ユーザー制限チェックトリガー
DROP TRIGGER IF EXISTS check_user_limit_trigger ON users;
CREATE TRIGGER check_user_limit_trigger
    BEFORE INSERT ON users
    FOR EACH ROW
    EXECUTE FUNCTION check_tenant_user_limit();

-- ログイン時刻更新関数
CREATE OR REPLACE FUNCTION update_last_login(user_uuid UUID)
RETURNS VOID AS $$
BEGIN
    UPDATE users 
    SET last_login_at = CURRENT_TIMESTAMP 
    WHERE id = user_uuid;
END;
$$ LANGUAGE plpgsql;

-- コメント追加
COMMENT ON TABLE users IS 'ユーザー管理テーブル - Kratosアイデンティティとの連携';
COMMENT ON COLUMN users.kratos_identity_id IS 'Ory Kratosのアイデンティティ ID';
COMMENT ON COLUMN users.preferences IS 'ユーザー個人設定 (JSON)';
COMMENT ON COLUMN users.last_login_at IS '最終ログイン時刻';
COMMENT ON COLUMN users.deleted_at IS 'ソフトデリート用タイムスタンプ';