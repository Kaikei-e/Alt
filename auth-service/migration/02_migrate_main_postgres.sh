#!/bin/bash

# ==========================================
# 02_migrate_main_postgres.sh
# Main-Postgres 段階的移行スクリプト
# ==========================================

set -e  # エラー時に停止
set -u  # 未定義変数使用時に停止

# 設定値
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCHEMA_DIR="${SCRIPT_DIR}/../schema"
LOG_FILE="${SCRIPT_DIR}/main_migration_$(date +%Y%m%d_%H%M%S).log"

# データベース接続設定
MAIN_DB_HOST="${MAIN_DB_HOST:-main-postgres}"
MAIN_DB_PORT="${MAIN_DB_PORT:-5432}"
MAIN_DB_NAME="${MAIN_DB_NAME:-alt_db}"
MAIN_DB_USER="${MAIN_DB_USER:-postgres}"
MAIN_DB_PASSWORD="${MAIN_DB_PASSWORD:-postgres_password}"

AUTH_DB_HOST="${AUTH_DB_HOST:-auth-postgres}"
AUTH_DB_PORT="${AUTH_DB_PORT:-5433}"
AUTH_DB_NAME="${AUTH_DB_NAME:-auth_db}"
AUTH_DB_USER="${AUTH_DB_USER:-auth_user}"
AUTH_DB_PASSWORD="${AUTH_DB_PASSWORD:-auth_password}"

# デフォルトユーザーID
DEFAULT_USER_ID="00000000-0000-0000-0000-000000000001"

# ドライランモード（実際の変更は行わない）
DRY_RUN="${DRY_RUN:-false}"

# ログ関数
log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') [INFO] $*" | tee -a "$LOG_FILE"
}

error() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') [ERROR] $*" | tee -a "$LOG_FILE" >&2
    exit 1
}

warning() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') [WARN] $*" | tee -a "$LOG_FILE"
}

# PostgreSQL接続チェック
check_connections() {
    log "Checking database connections..."
    
    # Main-Postgres接続チェック
    if ! PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "SELECT 1;" >/dev/null 2>&1; then
        error "Cannot connect to Main-PostgreSQL database"
    fi
    
    # Auth-Postgres接続チェック
    if ! PGPASSWORD="$AUTH_DB_PASSWORD" psql -h "$AUTH_DB_HOST" -p "$AUTH_DB_PORT" -U "$AUTH_DB_USER" -d "$AUTH_DB_NAME" -c "SELECT 1;" >/dev/null 2>&1; then
        error "Cannot connect to Auth-PostgreSQL database"
    fi
    
    log "Database connections verified"
}

# 移行前バックアップ
create_backup() {
    local backup_dir="${SCRIPT_DIR}/backup/$(date +%Y%m%d_%H%M%S)"
    mkdir -p "$backup_dir"
    
    log "Creating pre-migration backup..."
    
    # Main-postgres バックアップ
    if PGPASSWORD="$MAIN_DB_PASSWORD" pg_dump -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" "$MAIN_DB_NAME" > "$backup_dir/main_postgres_backup.sql" 2>/dev/null; then
        log "Main-postgres backup created: $backup_dir/main_postgres_backup.sql"
    else
        error "Failed to create main-postgres backup"
    fi
    
    # 重要テーブルの個別バックアップ
    local critical_tables=("read_status" "favorite_feeds" "feed_links" "articles" "feeds")
    
    for table in "${critical_tables[@]}"; do
        if PGPASSWORD="$MAIN_DB_PASSWORD" pg_dump -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -t "$table" > "$backup_dir/${table}_backup.sql" 2>/dev/null; then
            log "Table backup created: $table"
        else
            warning "Failed to backup table: $table"
        fi
    done
    
    echo "$backup_dir" > "${SCRIPT_DIR}/.last_backup_dir"
    log "Backup completed: $backup_dir"
}

# 既存データ状況確認
analyze_existing_data() {
    log "Analyzing existing data..."
    
    local tables=("read_status" "favorite_feeds" "feed_links")
    
    for table in "${tables[@]}"; do
        # テーブル存在チェック
        if ! PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "SELECT 1 FROM $table LIMIT 1;" >/dev/null 2>&1; then
            error "Required table does not exist: $table"
        fi
        
        # レコード数確認
        local count
        count=$(PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -t -c "SELECT COUNT(*) FROM $table;" 2>/dev/null | tr -d ' ')
        log "Table $table: $count records"
        
        # user_id カラム存在チェック
        if PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "SELECT user_id FROM $table LIMIT 1;" >/dev/null 2>&1; then
            local user_id_count
            user_id_count=$(PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -t -c "SELECT COUNT(*) FROM $table WHERE user_id IS NOT NULL;" 2>/dev/null | tr -d ' ')
            log "Table $table: user_id already exists, $user_id_count records with user_id"
        else
            log "Table $table: user_id column needs to be added"
        fi
    done
}

# Phase 1: カラム追加（NULL許可）
add_user_id_columns() {
    log "Phase 1: Adding user_id columns..."
    
    local tables=("read_status" "favorite_feeds" "feed_links")
    
    for table in "${tables[@]}"; do
        log "Adding user_id column to $table..."
        
        local sql="
        DO \$\$
        BEGIN
            IF NOT EXISTS (
                SELECT 1 FROM information_schema.columns 
                WHERE table_name = '$table' AND column_name = 'user_id'
            ) THEN
                ALTER TABLE $table ADD COLUMN user_id UUID;
                COMMENT ON COLUMN $table.user_id IS 'ユーザー ID (auth-postgres.users参照)';
            END IF;
        END \$\$;
        "
        
        if [[ "$DRY_RUN" == "true" ]]; then
            log "[DRY RUN] Would execute: ALTER TABLE $table ADD COLUMN user_id UUID"
        else
            if PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "$sql" >> "$LOG_FILE" 2>&1; then
                log "user_id column added to $table"
            else
                error "Failed to add user_id column to $table"
            fi
        fi
    done
}

# Phase 2: デフォルトユーザーIDでデータ更新
update_default_user_id() {
    log "Phase 2: Setting default user_id for existing data..."
    
    local tables=("read_status" "favorite_feeds" "feed_links")
    
    for table in "${tables[@]}"; do
        log "Updating $table with default user_id..."
        
        local sql="UPDATE $table SET user_id = '$DEFAULT_USER_ID' WHERE user_id IS NULL;"
        
        if [[ "$DRY_RUN" == "true" ]]; then
            local count
            count=$(PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -t -c "SELECT COUNT(*) FROM $table WHERE user_id IS NULL;" 2>/dev/null | tr -d ' ')
            log "[DRY RUN] Would update $count records in $table"
        else
            local updated_count
            updated_count=$(PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -t -c "$sql; SELECT ROW_COUNT();" 2>/dev/null | tail -1 | tr -d ' ')
            log "Updated $updated_count records in $table"
        fi
    done
}

# Phase 3: NOT NULL制約追加
add_not_null_constraints() {
    log "Phase 3: Adding NOT NULL constraints..."
    
    local tables=("read_status" "favorite_feeds" "feed_links")
    
    for table in "${tables[@]}"; do
        log "Adding NOT NULL constraint to $table.user_id..."
        
        local sql="ALTER TABLE $table ALTER COLUMN user_id SET NOT NULL;"
        
        if [[ "$DRY_RUN" == "true" ]]; then
            log "[DRY RUN] Would execute: $sql"
        else
            if PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "$sql" >> "$LOG_FILE" 2>&1; then
                log "NOT NULL constraint added to $table.user_id"
            else
                error "Failed to add NOT NULL constraint to $table.user_id"
            fi
        fi
    done
}

# Phase 4: インデックス追加
add_indexes() {
    log "Phase 4: Adding indexes..."
    
    local index_statements=(
        "CREATE INDEX IF NOT EXISTS idx_read_status_user_id ON read_status(user_id);"
        "CREATE INDEX IF NOT EXISTS idx_favorite_feeds_user_id ON favorite_feeds(user_id);"
        "CREATE INDEX IF NOT EXISTS idx_feed_links_user_id ON feed_links(user_id);"
        "CREATE INDEX IF NOT EXISTS idx_read_status_user_feed_read ON read_status(user_id, feed_id, is_read);"
        "CREATE INDEX IF NOT EXISTS idx_favorite_feeds_user_created ON favorite_feeds(user_id, created_at DESC);"
    )
    
    for sql in "${index_statements[@]}"; do
        local index_name
        index_name=$(echo "$sql" | grep -o 'idx_[a-z_]*' | head -1)
        
        log "Creating index: $index_name..."
        
        if [[ "$DRY_RUN" == "true" ]]; then
            log "[DRY RUN] Would execute: $sql"
        else
            if PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "$sql" >> "$LOG_FILE" 2>&1; then
                log "Index created: $index_name"
            else
                warning "Failed to create index: $index_name"
            fi
        fi
    done
}

# Phase 5: 一意制約更新
update_unique_constraints() {
    log "Phase 5: Updating unique constraints..."
    
    local constraint_statements=(
        "ALTER TABLE read_status DROP CONSTRAINT IF EXISTS read_status_feed_id_key;"
        "ALTER TABLE read_status ADD CONSTRAINT read_status_user_feed_unique UNIQUE(user_id, feed_id);"
        "ALTER TABLE favorite_feeds DROP CONSTRAINT IF EXISTS favorite_feeds_feed_id_key;"
        "ALTER TABLE favorite_feeds ADD CONSTRAINT favorite_feeds_user_feed_unique UNIQUE(user_id, feed_id);"
    )
    
    for sql in "${constraint_statements[@]}"; do
        log "Executing constraint update..."
        
        if [[ "$DRY_RUN" == "true" ]]; then
            log "[DRY RUN] Would execute: $sql"
        else
            if PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "$sql" >> "$LOG_FILE" 2>&1; then
                log "Constraint update completed"
            else
                warning "Constraint update failed (may not be critical): $sql"
            fi
        fi
    done
}

# Phase 6: 新規ユーザー固有テーブル作成
create_user_specific_tables() {
    log "Phase 6: Creating user-specific tables..."
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log "[DRY RUN] Would execute main_postgres_extensions.sql"
    else
        if PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -f "$SCHEMA_DIR/main_postgres_extensions.sql" >> "$LOG_FILE" 2>&1; then
            log "User-specific tables created successfully"
        else
            error "Failed to create user-specific tables"
        fi
    fi
}

# データ整合性チェック
verify_data_integrity() {
    log "Verifying data integrity after migration..."
    
    # 1. ユーザーIDが設定されているかチェック
    local tables=("read_status" "favorite_feeds" "feed_links")
    
    for table in "${tables[@]}"; do
        local total_count
        local with_user_id_count
        local without_user_id_count
        
        total_count=$(PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -t -c "SELECT COUNT(*) FROM $table;" 2>/dev/null | tr -d ' ')
        with_user_id_count=$(PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -t -c "SELECT COUNT(*) FROM $table WHERE user_id IS NOT NULL;" 2>/dev/null | tr -d ' ')
        without_user_id_count=$((total_count - with_user_id_count))
        
        log "Table $table: Total=$total_count, WithUserID=$with_user_id_count, WithoutUserID=$without_user_id_count"
        
        if [[ "$without_user_id_count" -gt 0 ]]; then
            error "Data integrity check failed: $table has $without_user_id_count records without user_id"
        fi
    done
    
    # 2. 重複チェック（新しい一意制約違反）
    for table in "${tables[@]}"; do
        local duplicate_count
        duplicate_count=$(PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -t -c "
        SELECT COUNT(*) FROM (
            SELECT user_id, feed_id, COUNT(*) 
            FROM $table 
            GROUP BY user_id, feed_id 
            HAVING COUNT(*) > 1
        ) duplicates;" 2>/dev/null | tr -d ' ')
        
        if [[ "$duplicate_count" -gt 0 ]]; then
            warning "Found $duplicate_count duplicate user_id+feed_id combinations in $table"
        else
            log "No duplicates found in $table"
        fi
    done
    
    log "Data integrity verification completed"
}

# 統計情報更新
update_statistics() {
    log "Updating database statistics..."
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log "[DRY RUN] Would update database statistics"
    else
        if PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "ANALYZE;" >> "$LOG_FILE" 2>&1; then
            log "Database statistics updated"
        else
            warning "Failed to update statistics (not critical)"
        fi
    fi
}

# メイン実行
main() {
    log "=== Main-Postgres Migration Started ==="
    log "Main DB: $MAIN_DB_HOST:$MAIN_DB_PORT/$MAIN_DB_NAME"
    log "Auth DB: $AUTH_DB_HOST:$AUTH_DB_PORT/$AUTH_DB_NAME"
    log "Default User ID: $DEFAULT_USER_ID"
    log "Dry Run: $DRY_RUN"
    log "Log file: $LOG_FILE"
    
    # 前処理
    check_connections
    analyze_existing_data
    
    if [[ "$DRY_RUN" != "true" ]]; then
        create_backup
    fi
    
    # 段階的移行実行
    add_user_id_columns
    update_default_user_id
    add_not_null_constraints
    add_indexes
    update_unique_constraints
    create_user_specific_tables
    
    # 後処理
    verify_data_integrity
    update_statistics
    
    log "=== Main-Postgres Migration Completed ==="
    log "Total time: $SECONDS seconds"
    
    echo ""
    if [[ "$DRY_RUN" == "true" ]]; then
        echo "🧪 Main-Postgres migration dry run completed successfully!"
        echo "   No actual changes were made to the database."
        echo "   Run without DRY_RUN=true to execute the migration."
    else
        echo "✅ Main-Postgres migration completed successfully!"
        echo "📝 Log file: $LOG_FILE"
        echo "💾 Backup: $(cat "${SCRIPT_DIR}/.last_backup_dir" 2>/dev/null || echo "Not available")"
    fi
    echo ""
    echo "Next steps:"
    echo "1. Verify migration results: 03_verify_migration.sh"
    echo "2. Set up cross-database integrity checks"
    echo "3. Update application connection strings"
}

# エラーハンドリング
trap 'error "Script interrupted"' INT TERM

# 実行
main "$@"