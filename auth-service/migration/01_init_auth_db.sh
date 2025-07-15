#!/bin/bash

# ==========================================
# 01_init_auth_db.sh
# Auth-Postgres データベース初期化スクリプト
# ==========================================

set -e  # エラー時に停止
set -u  # 未定義変数使用時に停止

# 設定値
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCHEMA_DIR="${SCRIPT_DIR}/../schema"
LOG_FILE="${SCRIPT_DIR}/migration_$(date +%Y%m%d_%H%M%S).log"

# データベース接続設定
AUTH_DB_HOST="${AUTH_DB_HOST:-auth-postgres}"
AUTH_DB_PORT="${AUTH_DB_PORT:-5433}"
AUTH_DB_NAME="${AUTH_DB_NAME:-auth_db}"
AUTH_DB_USER="${AUTH_DB_USER:-auth_user}"
AUTH_DB_PASSWORD="${AUTH_DB_PASSWORD:-auth_password}"

# ログ関数
log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') [INFO] $*" | tee -a "$LOG_FILE"
}

error() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') [ERROR] $*" | tee -a "$LOG_FILE" >&2
    exit 1
}

# PostgreSQL接続チェック
check_connection() {
    log "Checking PostgreSQL connection..."
    
    if ! PGPASSWORD="$AUTH_DB_PASSWORD" psql -h "$AUTH_DB_HOST" -p "$AUTH_DB_PORT" -U "$AUTH_DB_USER" -d "$AUTH_DB_NAME" -c "SELECT 1;" >/dev/null 2>&1; then
        error "Cannot connect to PostgreSQL database"
    fi
    
    log "PostgreSQL connection successful"
}

# スキーマファイル実行
execute_schema_file() {
    local file="$1"
    local description="$2"
    
    if [[ ! -f "$file" ]]; then
        error "Schema file not found: $file"
    fi
    
    log "Executing $description..."
    log "File: $(basename "$file")"
    
    if PGPASSWORD="$AUTH_DB_PASSWORD" psql -h "$AUTH_DB_HOST" -p "$AUTH_DB_PORT" -U "$AUTH_DB_USER" -d "$AUTH_DB_NAME" -f "$file" >> "$LOG_FILE" 2>&1; then
        log "$description completed successfully"
    else
        error "$description failed. Check log: $LOG_FILE"
    fi
}

# バックアップ作成
create_backup() {
    local backup_dir="${SCRIPT_DIR}/backup/$(date +%Y%m%d_%H%M%S)"
    mkdir -p "$backup_dir"
    
    log "Creating backup before initialization..."
    
    # データベース全体のバックアップ
    if PGPASSWORD="$AUTH_DB_PASSWORD" pg_dump -h "$AUTH_DB_HOST" -p "$AUTH_DB_PORT" -U "$AUTH_DB_USER" "$AUTH_DB_NAME" > "$backup_dir/auth_db_backup.sql" 2>/dev/null; then
        log "Backup created: $backup_dir/auth_db_backup.sql"
    else
        log "Backup skipped (database may not exist yet)"
    fi
}

# データベース初期化実行
initialize_database() {
    log "Starting Auth-Postgres database initialization..."
    
    # Phase 1: Core Tables
    execute_schema_file "$SCHEMA_DIR/01_tenants.sql" "Tenants table creation"
    execute_schema_file "$SCHEMA_DIR/02_users.sql" "Users table creation"
    
    # Phase 2: Session Management
    execute_schema_file "$SCHEMA_DIR/03_sessions.sql" "Session management tables creation"
    
    # Phase 3: Audit & Security
    execute_schema_file "$SCHEMA_DIR/04_audit.sql" "Audit and security tables creation"
    
    # Phase 4: Indexes & Optimization
    execute_schema_file "$SCHEMA_DIR/05_indexes.sql" "Indexes and optimization"
    
    log "Auth-Postgres database initialization completed successfully"
}

# 初期化後の検証
verify_initialization() {
    log "Verifying database initialization..."
    
    # テーブル存在チェック
    local expected_tables=(
        "tenants"
        "users"
        "user_sessions"
        "csrf_tokens"
        "audit_logs"
        "user_preferences"
    )
    
    for table in "${expected_tables[@]}"; do
        if PGPASSWORD="$AUTH_DB_PASSWORD" psql -h "$AUTH_DB_HOST" -p "$AUTH_DB_PORT" -U "$AUTH_DB_USER" -d "$AUTH_DB_NAME" -c "SELECT 1 FROM $table LIMIT 1;" >/dev/null 2>&1; then
            log "Table verified: $table"
        else
            error "Table verification failed: $table"
        fi
    done
    
    # デフォルトデータ確認
    local tenant_count
    tenant_count=$(PGPASSWORD="$AUTH_DB_PASSWORD" psql -h "$AUTH_DB_HOST" -p "$AUTH_DB_PORT" -U "$AUTH_DB_USER" -d "$AUTH_DB_NAME" -t -c "SELECT COUNT(*) FROM tenants;" 2>/dev/null | tr -d ' ')
    
    if [[ "$tenant_count" -ge 1 ]]; then
        log "Default tenant data verified: $tenant_count tenant(s)"
    else
        error "Default tenant data verification failed"
    fi
    
    local user_count
    user_count=$(PGPASSWORD="$AUTH_DB_PASSWORD" psql -h "$AUTH_DB_HOST" -p "$AUTH_DB_PORT" -U "$AUTH_DB_USER" -d "$AUTH_DB_NAME" -t -c "SELECT COUNT(*) FROM users;" 2>/dev/null | tr -d ' ')
    
    if [[ "$user_count" -ge 1 ]]; then
        log "Default user data verified: $user_count user(s)"
    else
        error "Default user data verification failed"
    fi
    
    log "Database initialization verification completed successfully"
}

# 統計情報更新
update_statistics() {
    log "Updating database statistics..."
    
    if PGPASSWORD="$AUTH_DB_PASSWORD" psql -h "$AUTH_DB_HOST" -p "$AUTH_DB_PORT" -U "$AUTH_DB_USER" -d "$AUTH_DB_NAME" -c "SELECT update_table_statistics();" >> "$LOG_FILE" 2>&1; then
        log "Database statistics updated"
    else
        log "Warning: Failed to update statistics (not critical)"
    fi
}

# メイン実行
main() {
    log "=== Auth-Postgres Database Initialization Started ==="
    log "Host: $AUTH_DB_HOST:$AUTH_DB_PORT"
    log "Database: $AUTH_DB_NAME"
    log "User: $AUTH_DB_USER"
    log "Log file: $LOG_FILE"
    
    # 前処理
    create_backup
    check_connection
    
    # 初期化実行
    initialize_database
    
    # 後処理
    verify_initialization
    update_statistics
    
    log "=== Auth-Postgres Database Initialization Completed ==="
    log "Total time: $SECONDS seconds"
    
    echo ""
    echo "✅ Auth-Postgres database initialization completed successfully!"
    echo "📝 Log file: $LOG_FILE"
    echo ""
    echo "Next steps:"
    echo "1. Run main-postgres migration: 02_migrate_main_postgres.sh"
    echo "2. Verify data integrity: 03_verify_migration.sh"
}

# エラーハンドリング
trap 'error "Script interrupted"' INT TERM

# 実行
main "$@"