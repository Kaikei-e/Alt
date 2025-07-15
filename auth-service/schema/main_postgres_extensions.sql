-- ==========================================
-- main_postgres_extensions.sql
-- Main-Postgres 拡張設計 - 既存テーブル拡張と新規ユーザー固有テーブル
-- ==========================================

-- ===== Phase 1: 既存テーブル拡張 =====

-- 1. read_status テーブル拡張
-- 既存テーブルにuser_id追加（段階的移行対応）
DO $$
BEGIN
    -- user_id カラムが存在しない場合のみ追加
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'read_status' AND column_name = 'user_id'
    ) THEN
        ALTER TABLE read_status ADD COLUMN user_id UUID;
        
        -- 外部キー制約（auth-postgresへの参照は後で設定）
        COMMENT ON COLUMN read_status.user_id IS 'ユーザー ID (auth-postgres.users参照)';
    END IF;
END $$;

-- 2. favorite_feeds テーブル拡張
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'favorite_feeds' AND column_name = 'user_id'
    ) THEN
        ALTER TABLE favorite_feeds ADD COLUMN user_id UUID;
        COMMENT ON COLUMN favorite_feeds.user_id IS 'ユーザー ID (auth-postgres.users参照)';
    END IF;
END $$;

-- 3. feed_links テーブル拡張（URL管理にユーザー固有設定追加）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'feed_links' AND column_name = 'user_id'
    ) THEN
        ALTER TABLE feed_links ADD COLUMN user_id UUID;
        COMMENT ON COLUMN feed_links.user_id IS 'ユーザー ID (auth-postgres.users参照)';
    END IF;
END $$;

-- ===== Phase 2: 新規ユーザー固有テーブル =====

-- 1. ユーザー別フィード設定テーブル
CREATE TABLE IF NOT EXISTS user_feed_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    feed_id UUID NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
    custom_name VARCHAR(255),
    notification_enabled BOOLEAN DEFAULT true,
    auto_mark_read BOOLEAN DEFAULT false,
    refresh_interval INTEGER DEFAULT 3600, -- 秒単位
    priority INTEGER DEFAULT 0 CHECK (priority >= 0 AND priority <= 10),
    tags JSONB DEFAULT '[]',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(user_id, feed_id)
    -- 外部キー制約（auth-postgresへの参照は後で設定）
    -- FOREIGN KEY (user_id) REFERENCES auth_postgres.users(id) ON DELETE CASCADE
);

-- インデックス
CREATE INDEX IF NOT EXISTS idx_user_feed_settings_user_id ON user_feed_settings(user_id);
CREATE INDEX IF NOT EXISTS idx_user_feed_settings_feed_id ON user_feed_settings(feed_id);
CREATE INDEX IF NOT EXISTS idx_user_feed_settings_priority ON user_feed_settings(user_id, priority DESC);
CREATE INDEX IF NOT EXISTS idx_user_feed_settings_notifications ON user_feed_settings(user_id) WHERE notification_enabled = true;

-- updated_at 自動更新トリガー
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS update_user_feed_settings_updated_at ON user_feed_settings;
CREATE TRIGGER update_user_feed_settings_updated_at
    BEFORE UPDATE ON user_feed_settings
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- 2. ユーザー別タグテーブル
CREATE TABLE IF NOT EXISTS user_tags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    name VARCHAR(100) NOT NULL,
    color VARCHAR(7) DEFAULT '#6366f1', -- デフォルト色（Indigo-500）
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(user_id, name)
    -- FOREIGN KEY (user_id) REFERENCES auth_postgres.users(id) ON DELETE CASCADE
);

-- インデックス
CREATE INDEX IF NOT EXISTS idx_user_tags_user_id ON user_tags(user_id);
CREATE INDEX IF NOT EXISTS idx_user_tags_name ON user_tags(user_id, name);

-- updated_at 自動更新トリガー
DROP TRIGGER IF EXISTS update_user_tags_updated_at ON user_tags;
CREATE TRIGGER update_user_tags_updated_at
    BEFORE UPDATE ON user_tags
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- 3. ユーザー別記事-タグ関連テーブル
CREATE TABLE IF NOT EXISTS user_article_tags (
    user_id UUID NOT NULL,
    article_id UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES user_tags(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY(user_id, article_id, tag_id)
    -- FOREIGN KEY (user_id) REFERENCES auth_postgres.users(id) ON DELETE CASCADE
);

-- インデックス
CREATE INDEX IF NOT EXISTS idx_user_article_tags_user_id ON user_article_tags(user_id);
CREATE INDEX IF NOT EXISTS idx_user_article_tags_article_id ON user_article_tags(article_id);
CREATE INDEX IF NOT EXISTS idx_user_article_tags_tag_id ON user_article_tags(tag_id);
CREATE INDEX IF NOT EXISTS idx_user_article_tags_user_article ON user_article_tags(user_id, article_id);

-- 4. ユーザー別記事メモ・評価テーブル
CREATE TABLE IF NOT EXISTS user_article_notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    article_id UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    note TEXT,
    rating INTEGER CHECK (rating >= 1 AND rating <= 5),
    is_bookmarked BOOLEAN DEFAULT false,
    read_later BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(user_id, article_id)
    -- FOREIGN KEY (user_id) REFERENCES auth_postgres.users(id) ON DELETE CASCADE
);

-- インデックス
CREATE INDEX IF NOT EXISTS idx_user_article_notes_user_id ON user_article_notes(user_id);
CREATE INDEX IF NOT EXISTS idx_user_article_notes_article_id ON user_article_notes(article_id);
CREATE INDEX IF NOT EXISTS idx_user_article_notes_bookmarked ON user_article_notes(user_id) WHERE is_bookmarked = true;
CREATE INDEX IF NOT EXISTS idx_user_article_notes_read_later ON user_article_notes(user_id) WHERE read_later = true;
CREATE INDEX IF NOT EXISTS idx_user_article_notes_rating ON user_article_notes(user_id, rating DESC) WHERE rating IS NOT NULL;

-- updated_at 自動更新トリガー
DROP TRIGGER IF EXISTS update_user_article_notes_updated_at ON user_article_notes;
CREATE TRIGGER update_user_article_notes_updated_at
    BEFORE UPDATE ON user_article_notes
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- 5. ユーザー別フォルダ・カテゴリテーブル
CREATE TABLE IF NOT EXISTS user_folders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    parent_id UUID REFERENCES user_folders(id) ON DELETE CASCADE,
    description TEXT,
    color VARCHAR(7) DEFAULT '#8b5cf6', -- デフォルト色（Violet-500）
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(user_id, name, parent_id)
    -- FOREIGN KEY (user_id) REFERENCES auth_postgres.users(id) ON DELETE CASCADE
);

-- インデックス
CREATE INDEX IF NOT EXISTS idx_user_folders_user_id ON user_folders(user_id);
CREATE INDEX IF NOT EXISTS idx_user_folders_parent_id ON user_folders(parent_id);
CREATE INDEX IF NOT EXISTS idx_user_folders_sort ON user_folders(user_id, sort_order);

-- updated_at 自動更新トリガー
DROP TRIGGER IF EXISTS update_user_folders_updated_at ON user_folders;
CREATE TRIGGER update_user_folders_updated_at
    BEFORE UPDATE ON user_folders
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- 6. フォルダ-フィード関連テーブル
CREATE TABLE IF NOT EXISTS user_folder_feeds (
    user_id UUID NOT NULL,
    folder_id UUID NOT NULL REFERENCES user_folders(id) ON DELETE CASCADE,
    feed_id UUID NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY(user_id, folder_id, feed_id)
    -- FOREIGN KEY (user_id) REFERENCES auth_postgres.users(id) ON DELETE CASCADE
);

-- インデックス
CREATE INDEX IF NOT EXISTS idx_user_folder_feeds_user_id ON user_folder_feeds(user_id);
CREATE INDEX IF NOT EXISTS idx_user_folder_feeds_folder_id ON user_folder_feeds(folder_id);
CREATE INDEX IF NOT EXISTS idx_user_folder_feeds_feed_id ON user_folder_feeds(feed_id);
CREATE INDEX IF NOT EXISTS idx_user_folder_feeds_sort ON user_folder_feeds(folder_id, sort_order);

-- ===== ヘルパー関数 =====

-- 1. ユーザー別フィード設定取得
CREATE OR REPLACE FUNCTION get_user_feed_settings(
    p_user_id UUID,
    p_feed_id UUID DEFAULT NULL
)
RETURNS TABLE(
    id UUID,
    feed_id UUID,
    custom_name VARCHAR(255),
    notification_enabled BOOLEAN,
    auto_mark_read BOOLEAN,
    refresh_interval INTEGER,
    priority INTEGER,
    tags JSONB
) AS $$
BEGIN
    IF p_feed_id IS NULL THEN
        -- 全フィード設定取得
        RETURN QUERY
        SELECT ufs.id, ufs.feed_id, ufs.custom_name, ufs.notification_enabled,
               ufs.auto_mark_read, ufs.refresh_interval, ufs.priority, ufs.tags
        FROM user_feed_settings ufs
        WHERE ufs.user_id = p_user_id
        ORDER BY ufs.priority DESC, ufs.custom_name;
    ELSE
        -- 特定フィード設定取得
        RETURN QUERY
        SELECT ufs.id, ufs.feed_id, ufs.custom_name, ufs.notification_enabled,
               ufs.auto_mark_read, ufs.refresh_interval, ufs.priority, ufs.tags
        FROM user_feed_settings ufs
        WHERE ufs.user_id = p_user_id AND ufs.feed_id = p_feed_id;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- 2. ユーザー別タグ一覧取得
CREATE OR REPLACE FUNCTION get_user_tags(p_user_id UUID)
RETURNS TABLE(
    id UUID,
    name VARCHAR(100),
    color VARCHAR(7),
    description TEXT,
    article_count BIGINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT ut.id, ut.name, ut.color, ut.description,
           COUNT(uat.article_id) as article_count
    FROM user_tags ut
    LEFT JOIN user_article_tags uat ON ut.id = uat.tag_id
    WHERE ut.user_id = p_user_id
    GROUP BY ut.id, ut.name, ut.color, ut.description
    ORDER BY ut.name;
END;
$$ LANGUAGE plpgsql;

-- 3. ユーザー別フォルダ階層取得
CREATE OR REPLACE FUNCTION get_user_folder_hierarchy(p_user_id UUID)
RETURNS TABLE(
    id UUID,
    name VARCHAR(255),
    parent_id UUID,
    level INTEGER,
    path TEXT,
    feed_count BIGINT
) AS $$
BEGIN
    RETURN QUERY
    WITH RECURSIVE folder_hierarchy AS (
        -- ルートフォルダ
        SELECT uf.id, uf.name, uf.parent_id, 0 as level, uf.name::TEXT as path
        FROM user_folders uf
        WHERE uf.user_id = p_user_id AND uf.parent_id IS NULL
        
        UNION ALL
        
        -- 子フォルダ
        SELECT uf.id, uf.name, uf.parent_id, fh.level + 1, 
               fh.path || '/' || uf.name
        FROM user_folders uf
        JOIN folder_hierarchy fh ON uf.parent_id = fh.id
        WHERE uf.user_id = p_user_id
    )
    SELECT fh.id, fh.name, fh.parent_id, fh.level, fh.path,
           COUNT(uff.feed_id) as feed_count
    FROM folder_hierarchy fh
    LEFT JOIN user_folder_feeds uff ON fh.id = uff.folder_id
    GROUP BY fh.id, fh.name, fh.parent_id, fh.level, fh.path
    ORDER BY fh.path;
END;
$$ LANGUAGE plpgsql;

-- 4. ユーザー統計情報取得
CREATE OR REPLACE FUNCTION get_user_stats(p_user_id UUID)
RETURNS TABLE(
    total_feeds BIGINT,
    total_articles BIGINT,
    read_articles BIGINT,
    unread_articles BIGINT,
    bookmarked_articles BIGINT,
    total_tags BIGINT,
    total_folders BIGINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        (SELECT COUNT(*) FROM user_feed_settings WHERE user_id = p_user_id) as total_feeds,
        (SELECT COUNT(*) FROM read_status rs 
         JOIN articles a ON rs.article_id = a.id 
         WHERE rs.user_id = p_user_id) as total_articles,
        (SELECT COUNT(*) FROM read_status WHERE user_id = p_user_id AND is_read = true) as read_articles,
        (SELECT COUNT(*) FROM read_status WHERE user_id = p_user_id AND is_read = false) as unread_articles,
        (SELECT COUNT(*) FROM user_article_notes WHERE user_id = p_user_id AND is_bookmarked = true) as bookmarked_articles,
        (SELECT COUNT(*) FROM user_tags WHERE user_id = p_user_id) as total_tags,
        (SELECT COUNT(*) FROM user_folders WHERE user_id = p_user_id) as total_folders;
END;
$$ LANGUAGE plpgsql;

-- ===== データ整合性チェック関数 =====

-- クロスデータベース整合性チェック（一時的な実装）
CREATE OR REPLACE FUNCTION check_user_exists_stub(user_uuid UUID)
RETURNS BOOLEAN AS $$
BEGIN
    -- 実際の実装では auth-postgres の users テーブルをチェック
    -- 現在はデフォルトユーザーのみ存在として扱う
    RETURN user_uuid = '00000000-0000-0000-0000-000000000001'::UUID 
           OR user_uuid IS NULL;
END;
$$ LANGUAGE plpgsql;

-- ユーザー参照整合性チェック
CREATE OR REPLACE FUNCTION validate_user_reference()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.user_id IS NOT NULL AND NOT check_user_exists_stub(NEW.user_id) THEN
        RAISE EXCEPTION 'User ID % does not exist in auth database', NEW.user_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ===== バリデーション関数 =====

-- タグ配列のバリデーション
CREATE OR REPLACE FUNCTION validate_tags_array(tags JSONB)
RETURNS BOOLEAN AS $$
BEGIN
    -- 空配列は許可
    IF tags = '[]'::JSONB THEN
        RETURN TRUE;
    END IF;
    
    -- 配列形式チェック
    IF jsonb_typeof(tags) != 'array' THEN
        RETURN FALSE;
    END IF;
    
    -- 各要素が文字列かチェック
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements(tags) elem
        WHERE jsonb_typeof(elem) != 'string'
    ) THEN
        RETURN FALSE;
    END IF;
    
    RETURN TRUE;
END;
$$ LANGUAGE plpgsql;

-- フィード設定のタグ配列バリデーション制約
ALTER TABLE user_feed_settings 
ADD CONSTRAINT valid_tags_array 
CHECK (validate_tags_array(tags));

-- 色コードバリデーション
ALTER TABLE user_tags 
ADD CONSTRAINT valid_color_code 
CHECK (color ~ '^#[0-9a-fA-F]{6}$');

ALTER TABLE user_folders 
ADD CONSTRAINT valid_folder_color_code 
CHECK (color ~ '^#[0-9a-fA-F]{6}$');

-- ===== コメント追加 =====

COMMENT ON TABLE user_feed_settings IS 'ユーザー別フィード設定テーブル';
COMMENT ON TABLE user_tags IS 'ユーザー別タグテーブル';
COMMENT ON TABLE user_article_tags IS 'ユーザー別記事-タグ関連テーブル';
COMMENT ON TABLE user_article_notes IS 'ユーザー別記事メモ・評価テーブル';
COMMENT ON TABLE user_folders IS 'ユーザー別フォルダ・カテゴリテーブル';
COMMENT ON TABLE user_folder_feeds IS 'フォルダ-フィード関連テーブル';

-- カラムコメント
COMMENT ON COLUMN user_feed_settings.refresh_interval IS 'フィード更新間隔（秒）';
COMMENT ON COLUMN user_feed_settings.priority IS 'フィード優先度（0-10、高いほど優先）';
COMMENT ON COLUMN user_feed_settings.tags IS 'フィード用タグ配列（JSON）';
COMMENT ON COLUMN user_tags.color IS 'タグ表示色（Hex形式 #RRGGBB）';
COMMENT ON COLUMN user_article_notes.rating IS '記事評価（1-5点）';
COMMENT ON COLUMN user_folders.sort_order IS 'フォルダ表示順序';

-- ===== 初期データ投入 =====

-- デフォルトユーザー用の初期設定
DO $$
DECLARE
    default_user_id UUID := '00000000-0000-0000-0000-000000000001';
    default_tag_id UUID;
    default_folder_id UUID;
BEGIN
    -- デフォルトタグ作成
    INSERT INTO user_tags (id, user_id, name, color, description)
    VALUES 
        (gen_random_uuid(), default_user_id, 'Important', '#ef4444', 'Important articles'),
        (gen_random_uuid(), default_user_id, 'Tech', '#3b82f6', 'Technology related'),
        (gen_random_uuid(), default_user_id, 'News', '#10b981', 'News articles')
    ON CONFLICT (user_id, name) DO NOTHING;
    
    -- デフォルトフォルダ作成
    INSERT INTO user_folders (id, user_id, name, description, color)
    VALUES 
        (gen_random_uuid(), default_user_id, 'Tech News', 'Technology and development news', '#6366f1'),
        (gen_random_uuid(), default_user_id, 'Personal', 'Personal interest feeds', '#8b5cf6')
    ON CONFLICT (user_id, name, parent_id) DO NOTHING;
END $$;