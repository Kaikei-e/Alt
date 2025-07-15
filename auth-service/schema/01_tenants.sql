-- ==========================================
-- 01_tenants.sql
-- テナント管理テーブル作成
-- ==========================================

-- テナント管理テーブル
CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug VARCHAR(100) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(20) DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleted')),
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE NULL
);

-- インデックス
CREATE INDEX IF NOT EXISTS idx_tenants_slug ON tenants(slug);
CREATE INDEX IF NOT EXISTS idx_tenants_status ON tenants(status);
CREATE INDEX IF NOT EXISTS idx_tenants_created_at ON tenants(created_at);
CREATE INDEX IF NOT EXISTS idx_tenants_deleted_at ON tenants(deleted_at) WHERE deleted_at IS NOT NULL;

-- updated_at の自動更新トリガー
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- トリガー作成
DROP TRIGGER IF EXISTS update_tenants_updated_at ON tenants;
CREATE TRIGGER update_tenants_updated_at
    BEFORE UPDATE ON tenants
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- テナント設定のJSON検証関数
CREATE OR REPLACE FUNCTION validate_tenant_settings(settings JSONB)
RETURNS BOOLEAN AS $$
BEGIN
    -- 必須フィールドの存在チェック
    IF NOT (settings ? 'features' AND settings ? 'limits') THEN
        RETURN FALSE;
    END IF;
    
    -- features配列の検証
    IF NOT (jsonb_typeof(settings->'features') = 'array') THEN
        RETURN FALSE;
    END IF;
    
    -- limits オブジェクトの検証
    IF NOT (jsonb_typeof(settings->'limits') = 'object') THEN
        RETURN FALSE;
    END IF;
    
    -- max_feeds と max_users の存在チェック
    IF NOT (settings->'limits' ? 'max_feeds' AND settings->'limits' ? 'max_users') THEN
        RETURN FALSE;
    END IF;
    
    RETURN TRUE;
END;
$$ LANGUAGE plpgsql;

-- 設定バリデーション制約
ALTER TABLE tenants 
ADD CONSTRAINT valid_tenant_settings 
CHECK (validate_tenant_settings(settings));

-- デフォルトテナント作成
INSERT INTO tenants (id, slug, name, status, settings)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'default',
    'Default Tenant',
    'active',
    '{
        "features": ["rss_feeds", "ai_summary", "tags"],
        "limits": {"max_feeds": 10000, "max_users": 1},
        "timezone": "UTC",
        "language": "en"
    }'::JSONB
) ON CONFLICT (id) DO NOTHING;

-- テナント関連のヘルパー関数
CREATE OR REPLACE FUNCTION get_tenant_by_slug(tenant_slug VARCHAR(100))
RETURNS tenants AS $$
DECLARE
    tenant_record tenants;
BEGIN
    SELECT * INTO tenant_record 
    FROM tenants 
    WHERE slug = tenant_slug AND deleted_at IS NULL;
    
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Tenant with slug % not found', tenant_slug;
    END IF;
    
    RETURN tenant_record;
END;
$$ LANGUAGE plpgsql;

-- テナントアクティブ状態チェック関数
CREATE OR REPLACE FUNCTION is_tenant_active(tenant_uuid UUID)
RETURNS BOOLEAN AS $$
DECLARE
    tenant_status VARCHAR(20);
BEGIN
    SELECT status INTO tenant_status
    FROM tenants
    WHERE id = tenant_uuid AND deleted_at IS NULL;
    
    RETURN tenant_status = 'active';
END;
$$ LANGUAGE plpgsql;

-- コメント追加
COMMENT ON TABLE tenants IS 'テナント管理テーブル - マルチテナント対応の基盤';
COMMENT ON COLUMN tenants.slug IS 'テナント識別子 (URL friendly)';
COMMENT ON COLUMN tenants.settings IS 'テナント固有設定 (JSON)';
COMMENT ON COLUMN tenants.deleted_at IS 'ソフトデリート用タイムスタンプ';