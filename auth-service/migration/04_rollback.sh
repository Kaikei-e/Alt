#!/bin/bash

# ==========================================
# 04_rollback.sh
# 移行ロールバック（緊急時復旧）スクリプト
# ==========================================

set -e  # エラー時に停止
set -u  # 未定義変数使用時に停止

# 設定値
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_FILE="${SCRIPT_DIR}/rollback_$(date +%Y%m%d_%H%M%S).log"

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

# ロールバックレベル
ROLLBACK_LEVEL="${ROLLBACK_LEVEL:-partial}"  # partial, full, backup_restore

# ドライランモード
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

# 確認プロンプト
confirm() {
    local message="$1"
    local default="${2:-n}"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log "[DRY RUN] Would prompt: $message"
        return 0
    fi
    
    echo -n "$message [y/N]: "
    read -r response
    response=${response:-$default}
    
    if [[ "$response" =~ ^[Yy]$ ]]; then
        return 0
    else
        return 1
    fi
}

# PostgreSQL接続チェック
check_connections() {
    log "Checking database connections..."
    
    if ! PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "SELECT 1;" >/dev/null 2>&1; then
        error "Cannot connect to Main-PostgreSQL database"
    fi
    
    log "Database connections verified"
}

# バックアップ確認
verify_backup() {
    local backup_dir_file="${SCRIPT_DIR}/.last_backup_dir"
    
    if [[ -f "$backup_dir_file" ]]; then
        local backup_dir
        backup_dir=$(cat "$backup_dir_file")
        
        if [[ -d "$backup_dir" ]]; then
            log "Backup directory found: $backup_dir"
            
            if [[ -f "$backup_dir/main_postgres_backup.sql" ]]; then
                log "Main database backup file exists"
                return 0
            else
                warning "Main database backup file not found"
            fi
        else
            warning "Backup directory does not exist: $backup_dir"
        fi
    else
        warning "No backup directory reference found"
    fi
    
    return 1
}

# 現在の状態チェック
check_current_state() {
    log "Checking current database state..."
    
    # user_id カラムの存在チェック
    local tables=("read_status" "favorite_feeds" "feed_links")
    local migrated_tables=0
    
    for table in "${tables[@]}"; do
        if PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "SELECT user_id FROM $table LIMIT 1;" >/dev/null 2>&1; then
            log "Table $table has user_id column (migrated)"
            ((migrated_tables++))
        else
            log "Table $table does not have user_id column (not migrated)"
        fi
    done
    
    if [[ "$migrated_tables" -eq 0 ]]; then
        log "No migration detected - nothing to rollback"
        return 1
    elif [[ "$migrated_tables" -eq 3 ]]; then
        log "Full migration detected - rollback possible"
        return 0
    else
        warning "Partial migration detected - $migrated_tables/3 tables migrated"
        return 0
    fi
}

# 部分ロールバック（スキーマのみ）
partial_rollback() {
    log "Starting partial rollback (schema changes only)..."
    
    if ! confirm "This will remove user_id columns and related constraints. Continue?"; then
        log "Rollback cancelled by user"
        return 1
    fi
    
    # ロールバック前バックアップ
    create_rollback_backup
    
    # 制約削除
    log "Removing constraints..."
    local constraint_statements=(
        "ALTER TABLE read_status DROP CONSTRAINT IF EXISTS read_status_user_feed_unique;"
        "ALTER TABLE favorite_feeds DROP CONSTRAINT IF EXISTS favorite_feeds_user_feed_unique;"
    )
    
    for sql in "${constraint_statements[@]}"; do
        if [[ "$DRY_RUN" == "true" ]]; then
            log "[DRY RUN] Would execute: $sql"
        else
            if PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "$sql" >> "$LOG_FILE" 2>&1; then
                log "Constraint removed successfully"
            else
                warning "Failed to remove constraint (may not exist): $sql"
            fi
        fi
    done
    
    # インデックス削除
    log "Removing indexes..."
    local index_statements=(
        "DROP INDEX IF EXISTS idx_read_status_user_id;"
        "DROP INDEX IF EXISTS idx_favorite_feeds_user_id;"
        "DROP INDEX IF EXISTS idx_feed_links_user_id;"
        "DROP INDEX IF EXISTS idx_read_status_user_feed_read;"
        "DROP INDEX IF EXISTS idx_favorite_feeds_user_created;"
    )
    
    for sql in "${index_statements[@]}"; do
        if [[ "$DRY_RUN" == "true" ]]; then
            log "[DRY RUN] Would execute: $sql"
        else
            if PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "$sql" >> "$LOG_FILE" 2>&1; then
                log "Index removed successfully"
            else
                warning "Failed to remove index (may not exist): $sql"
            fi
        fi
    done
    
    # user_id カラム削除
    log "Removing user_id columns..."
    local tables=("read_status" "favorite_feeds" "feed_links")
    
    for table in "${tables[@]}"; do
        local sql="ALTER TABLE $table DROP COLUMN IF EXISTS user_id;"
        
        if [[ "$DRY_RUN" == "true" ]]; then
            log "[DRY RUN] Would execute: $sql"
        else
            if PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "$sql" >> "$LOG_FILE" 2>&1; then
                log "user_id column removed from $table"
            else
                error "Failed to remove user_id column from $table"
            fi
        fi
    done
    
    # ユーザー固有テーブル削除
    log "Removing user-specific tables..."
    local user_tables=(
        "user_folder_feeds"
        "user_folders"
        "user_article_notes"
        "user_article_tags"
        "user_tags"
        "user_feed_settings"
    )
    
    for table in "${user_tables[@]}"; do
        local sql="DROP TABLE IF EXISTS $table CASCADE;"
        
        if [[ "$DRY_RUN" == "true" ]]; then
            log "[DRY RUN] Would execute: $sql"
        else
            if PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "$sql" >> "$LOG_FILE" 2>&1; then
                log "User table removed: $table"
            else
                warning "Failed to remove user table: $table"
            fi
        fi
    done
    
    log "Partial rollback completed"
}

# 完全ロールバック（Auth DBも含む）
full_rollback() {
    log "Starting full rollback (including Auth-Postgres)..."
    
    if ! confirm "This will remove ALL migration changes including Auth-Postgres. Continue?"; then
        log "Rollback cancelled by user"
        return 1
    fi
    
    # Main-Postgres の部分ロールバック実行
    partial_rollback
    
    # Auth-Postgres データベース削除
    log "Removing Auth-Postgres database..."
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log "[DRY RUN] Would drop Auth-Postgres database"
    else
        # 接続終了
        if PGPASSWORD="$AUTH_DB_PASSWORD" psql -h "$AUTH_DB_HOST" -p "$AUTH_DB_PORT" -U "$AUTH_DB_USER" -d "postgres" -c "
        SELECT pg_terminate_backend(pid) 
        FROM pg_stat_activity 
        WHERE datname = '$AUTH_DB_NAME' AND pid <> pg_backend_pid();
        " >> "$LOG_FILE" 2>&1; then
            log "Terminated existing connections to Auth database"
        fi
        
        # データベース削除
        if PGPASSWORD="$AUTH_DB_PASSWORD" psql -h "$AUTH_DB_HOST" -p "$AUTH_DB_PORT" -U "$AUTH_DB_USER" -d "postgres" -c "DROP DATABASE IF EXISTS $AUTH_DB_NAME;" >> "$LOG_FILE" 2>&1; then
            log "Auth-Postgres database removed"
        else
            warning "Failed to remove Auth-Postgres database"
        fi
    fi
    
    log "Full rollback completed"
}

# バックアップ復元
backup_restore() {
    log "Starting backup restore..."
    
    if ! verify_backup; then
        error "No valid backup found for restore"
    fi
    
    local backup_dir
    backup_dir=$(cat "${SCRIPT_DIR}/.last_backup_dir")
    local backup_file="$backup_dir/main_postgres_backup.sql"
    
    if ! confirm "This will restore database from backup: $backup_file. Continue?"; then
        log "Backup restore cancelled by user"
        return 1
    fi
    
    # 現在の状態をバックアップ
    create_rollback_backup
    
    # データベース復元
    log "Restoring database from backup..."
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log "[DRY RUN] Would restore from: $backup_file"
    else
        # 接続終了
        if PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "postgres" -c "
        SELECT pg_terminate_backend(pid) 
        FROM pg_stat_activity 
        WHERE datname = '$MAIN_DB_NAME' AND pid <> pg_backend_pid();
        " >> "$LOG_FILE" 2>&1; then
            log "Terminated existing connections"
        fi
        
        # データベース再作成
        PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "postgres" -c "
        DROP DATABASE IF EXISTS $MAIN_DB_NAME;
        CREATE DATABASE $MAIN_DB_NAME;
        " >> "$LOG_FILE" 2>&1
        
        # バックアップ復元
        if PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -f "$backup_file" >> "$LOG_FILE" 2>&1; then
            log "Database restored from backup successfully"
        else
            error "Failed to restore database from backup"
        fi
    fi
    
    log "Backup restore completed"
}

# ロールバック前バックアップ作成
create_rollback_backup() {
    local backup_dir="${SCRIPT_DIR}/rollback_backup/$(date +%Y%m%d_%H%M%S)"
    mkdir -p "$backup_dir"
    
    log "Creating rollback backup..."
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log "[DRY RUN] Would create rollback backup: $backup_dir"
    else
        if PGPASSWORD="$MAIN_DB_PASSWORD" pg_dump -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" "$MAIN_DB_NAME" > "$backup_dir/pre_rollback_backup.sql" 2>/dev/null; then
            log "Rollback backup created: $backup_dir/pre_rollback_backup.sql"
        else
            warning "Failed to create rollback backup"
        fi
    fi
}

# ロールバック後検証
verify_rollback() {
    log "Verifying rollback results..."
    
    local tables=("read_status" "favorite_feeds" "feed_links")
    local rollback_success=true
    
    for table in "${tables[@]}"; do
        if PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "SELECT user_id FROM $table LIMIT 1;" >/dev/null 2>&1; then
            warning "Table $table still has user_id column"
            rollback_success=false
        else
            log "Table $table: user_id column removed"
        fi
    done
    
    if [[ "$rollback_success" == "true" ]]; then
        log "Rollback verification successful"
        return 0
    else
        warning "Rollback verification found issues"
        return 1
    fi
}

# 使用方法表示
show_usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Rollback migration changes in case of issues.

Options:
  -l, --level LEVEL     Rollback level: partial, full, backup_restore (default: partial)
  -d, --dry-run         Dry run mode - show what would be done without making changes
  -h, --help           Show this help message

Rollback Levels:
  partial              Remove user_id columns and user-specific tables (recommended)
  full                 Remove all migration changes including Auth-Postgres
  backup_restore       Restore from pre-migration backup (destructive)

Environment Variables:
  MAIN_DB_HOST         Main database host (default: main-postgres)
  MAIN_DB_PORT         Main database port (default: 5432)
  MAIN_DB_NAME         Main database name (default: alt_db)
  MAIN_DB_USER         Main database user (default: postgres)
  MAIN_DB_PASSWORD     Main database password (required)
  
  AUTH_DB_HOST         Auth database host (default: auth-postgres)
  AUTH_DB_PORT         Auth database port (default: 5433)
  AUTH_DB_NAME         Auth database name (default: auth_db)
  AUTH_DB_USER         Auth database user (default: auth_user)
  AUTH_DB_PASSWORD     Auth database password (required)

Examples:
  # Dry run partial rollback
  DRY_RUN=true $0 --level partial
  
  # Execute partial rollback
  $0 --level partial
  
  # Full rollback including Auth-Postgres
  $0 --level full
  
  # Restore from backup
  $0 --level backup_restore
EOF
}

# コマンドライン引数解析
parse_arguments() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -l|--level)
                ROLLBACK_LEVEL="$2"
                shift 2
                ;;
            -d|--dry-run)
                DRY_RUN="true"
                shift
                ;;
            -h|--help)
                show_usage
                exit 0
                ;;
            *)
                error "Unknown option: $1"
                ;;
        esac
    done
    
    # Validation
    if [[ ! "$ROLLBACK_LEVEL" =~ ^(partial|full|backup_restore)$ ]]; then
        error "Invalid rollback level: $ROLLBACK_LEVEL"
    fi
}

# メイン実行
main() {
    parse_arguments "$@"
    
    log "=== Migration Rollback Started ==="
    log "Rollback Level: $ROLLBACK_LEVEL"
    log "Dry Run: $DRY_RUN"
    log "Log file: $LOG_FILE"
    
    # 警告表示
    cat << EOF

⚠️  WARNING: Database Rollback Operation ⚠️

This operation will modify or remove migration changes.
Make sure you understand the consequences:

- Level: $ROLLBACK_LEVEL
- Dry Run: $DRY_RUN
- This operation may result in data loss
- Always ensure you have current backups

EOF
    
    if [[ "$DRY_RUN" != "true" ]] && ! confirm "Do you want to continue with the rollback?"; then
        log "Rollback cancelled by user"
        exit 0
    fi
    
    # 前処理
    check_connections
    
    if ! check_current_state; then
        log "No migration changes detected - nothing to rollback"
        exit 0
    fi
    
    # ロールバック実行
    case "$ROLLBACK_LEVEL" in
        "partial")
            partial_rollback
            ;;
        "full")
            full_rollback
            ;;
        "backup_restore")
            backup_restore
            ;;
    esac
    
    # 後処理
    if [[ "$DRY_RUN" != "true" ]]; then
        verify_rollback
    fi
    
    log "=== Migration Rollback Completed ==="
    log "Total time: $SECONDS seconds"
    
    echo ""
    if [[ "$DRY_RUN" == "true" ]]; then
        echo "🧪 Rollback dry run completed successfully!"
        echo "   No actual changes were made to the database."
        echo "   Run without --dry-run to execute the rollback."
    else
        echo "✅ Rollback completed successfully!"
        echo "📝 Log file: $LOG_FILE"
    fi
    echo ""
    echo "Next steps:"
    echo "1. Verify application functionality"
    echo "2. Update application configuration if needed"
    echo "3. Consider re-running migration after fixing issues"
}

# エラーハンドリング
trap 'error "Rollback interrupted"' INT TERM

# 実行
main "$@"