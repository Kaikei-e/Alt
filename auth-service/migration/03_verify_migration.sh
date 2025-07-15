#!/bin/bash

# ==========================================
# 03_verify_migration.sh
# データ移行検証・整合性チェックスクリプト
# ==========================================

set -e  # エラー時に停止
set -u  # 未定義変数使用時に停止

# 設定値
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_FILE="${SCRIPT_DIR}/verification_$(date +%Y%m%d_%H%M%S).log"

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

# テスト結果
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_WARNINGS=0

# ログ関数
log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') [INFO] $*" | tee -a "$LOG_FILE"
}

error() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') [ERROR] $*" | tee -a "$LOG_FILE" >&2
}

warning() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') [WARN] $*" | tee -a "$LOG_FILE"
}

success() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') [PASS] $*" | tee -a "$LOG_FILE"
}

# テスト結果記録
test_pass() {
    ((TESTS_PASSED++))
    success "$1"
}

test_fail() {
    ((TESTS_FAILED++))
    error "$1"
}

test_warn() {
    ((TESTS_WARNINGS++))
    warning "$1"
}

# PostgreSQL クエリ実行（Main DB）
query_main() {
    PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -t -c "$1" 2>/dev/null | tr -d ' '
}

# PostgreSQL クエリ実行（Auth DB）
query_auth() {
    PGPASSWORD="$AUTH_DB_PASSWORD" psql -h "$AUTH_DB_HOST" -p "$AUTH_DB_PORT" -U "$AUTH_DB_USER" -d "$AUTH_DB_NAME" -t -c "$1" 2>/dev/null | tr -d ' '
}

# 接続チェック
check_connections() {
    log "Testing database connections..."
    
    if query_main "SELECT 1;" >/dev/null 2>&1; then
        test_pass "Main-Postgres connection successful"
    else
        test_fail "Main-Postgres connection failed"
        return 1
    fi
    
    if query_auth "SELECT 1;" >/dev/null 2>&1; then
        test_pass "Auth-Postgres connection successful"
    else
        test_fail "Auth-Postgres connection failed"
        return 1
    fi
}

# スキーマ構造チェック
verify_schema_structure() {
    log "Verifying schema structure..."
    
    # Auth-Postgres テーブル存在チェック
    local auth_tables=("tenants" "users" "user_sessions" "csrf_tokens" "audit_logs" "user_preferences")
    
    for table in "${auth_tables[@]}"; do
        if query_auth "SELECT 1 FROM $table LIMIT 1;" >/dev/null 2>&1; then
            test_pass "Auth table exists: $table"
        else
            test_fail "Auth table missing: $table"
        fi
    done
    
    # Main-Postgres user_id カラム存在チェック
    local main_tables=("read_status" "favorite_feeds" "feed_links")
    
    for table in "${main_tables[@]}"; do
        if query_main "SELECT user_id FROM $table LIMIT 1;" >/dev/null 2>&1; then
            test_pass "Main table has user_id column: $table"
        else
            test_fail "Main table missing user_id column: $table"
        fi
    done
    
    # 新規ユーザー固有テーブル存在チェック
    local user_tables=("user_feed_settings" "user_tags" "user_article_tags" "user_article_notes" "user_folders" "user_folder_feeds")
    
    for table in "${user_tables[@]}"; do
        if query_main "SELECT 1 FROM $table LIMIT 1;" >/dev/null 2>&1; then
            test_pass "User-specific table exists: $table"
        else
            test_fail "User-specific table missing: $table"
        fi
    done
}

# データ整合性チェック
verify_data_integrity() {
    log "Verifying data integrity..."
    
    # 1. ユーザーIDが設定されているかチェック
    local tables=("read_status" "favorite_feeds" "feed_links")
    
    for table in "${tables[@]}"; do
        local total_count
        local with_user_id_count
        local without_user_id_count
        
        total_count=$(query_main "SELECT COUNT(*) FROM $table;")
        with_user_id_count=$(query_main "SELECT COUNT(*) FROM $table WHERE user_id IS NOT NULL;")
        without_user_id_count=$((total_count - with_user_id_count))
        
        log "Table $table: Total=$total_count, WithUserID=$with_user_id_count, WithoutUserID=$without_user_id_count"
        
        if [[ "$without_user_id_count" -eq 0 ]]; then
            test_pass "All records in $table have user_id"
        else
            test_fail "$table has $without_user_id_count records without user_id"
        fi
    done
    
    # 2. 外部キー整合性チェック（デフォルトユーザーの存在確認）
    local default_user_exists
    default_user_exists=$(query_auth "SELECT COUNT(*) FROM users WHERE id = '$DEFAULT_USER_ID';")
    
    if [[ "$default_user_exists" -eq 1 ]]; then
        test_pass "Default user exists in auth database"
    else
        test_fail "Default user missing in auth database"
    fi
    
    # 3. 重複データチェック
    for table in "${tables[@]}"; do
        local duplicate_count
        duplicate_count=$(query_main "
        SELECT COUNT(*) FROM (
            SELECT user_id, feed_id, COUNT(*) 
            FROM $table 
            GROUP BY user_id, feed_id 
            HAVING COUNT(*) > 1
        ) duplicates;")
        
        if [[ "$duplicate_count" -eq 0 ]]; then
            test_pass "No duplicates in $table"
        else
            test_warn "Found $duplicate_count duplicate user_id+feed_id combinations in $table"
        fi
    done
}

# インデックス存在チェック
verify_indexes() {
    log "Verifying indexes..."
    
    # 重要なインデックスの存在チェック
    local main_indexes=(
        "idx_read_status_user_id"
        "idx_favorite_feeds_user_id"
        "idx_feed_links_user_id"
        "idx_user_feed_settings_user_id"
        "idx_user_tags_user_id"
    )
    
    for index in "${main_indexes[@]}"; do
        if query_main "SELECT 1 FROM pg_indexes WHERE indexname = '$index';" >/dev/null 2>&1; then
            test_pass "Index exists: $index"
        else
            test_fail "Index missing: $index"
        fi
    done
    
    # Auth-Postgres インデックス
    local auth_indexes=(
        "idx_users_tenant_id"
        "idx_users_kratos_identity_id"
        "idx_user_sessions_user_id"
        "idx_csrf_tokens_session_id"
    )
    
    for index in "${auth_indexes[@]}"; do
        if query_auth "SELECT 1 FROM pg_indexes WHERE indexname = '$index';" >/dev/null 2>&1; then
            test_pass "Auth index exists: $index"
        else
            test_fail "Auth index missing: $index"
        fi
    done
}

# 制約チェック
verify_constraints() {
    log "Verifying constraints..."
    
    # Main-Postgres の一意制約
    local main_constraints=(
        "read_status_user_feed_unique"
        "favorite_feeds_user_feed_unique"
    )
    
    for constraint in "${main_constraints[@]}"; do
        if query_main "SELECT 1 FROM pg_constraint WHERE conname = '$constraint';" >/dev/null 2>&1; then
            test_pass "Constraint exists: $constraint"
        else
            test_warn "Constraint missing: $constraint"
        fi
    done
    
    # Auth-Postgres の制約
    local auth_constraints=(
        "tenants_slug_key"
        "users_kratos_identity_id_key"
        "users_tenant_id_email_key"
    )
    
    for constraint in "${auth_constraints[@]}"; do
        if query_auth "SELECT 1 FROM pg_constraint WHERE conname = '$constraint';" >/dev/null 2>&1; then
            test_pass "Auth constraint exists: $constraint"
        else
            test_fail "Auth constraint missing: $constraint"
        fi
    done
}

# パフォーマンステスト
verify_performance() {
    log "Running performance tests..."
    
    # クエリ実行時間測定
    local start_time
    local end_time
    local duration
    
    # 1. ユーザー別読書状況取得
    start_time=$(date +%s%N)
    query_main "SELECT COUNT(*) FROM read_status WHERE user_id = '$DEFAULT_USER_ID' AND is_read = true;" >/dev/null
    end_time=$(date +%s%N)
    duration=$(((end_time - start_time) / 1000000))  # nanoseconds to milliseconds
    
    if [[ "$duration" -lt 1000 ]]; then  # 1秒未満
        test_pass "Read status query performance: ${duration}ms"
    else
        test_warn "Read status query slow: ${duration}ms"
    fi
    
    # 2. ユーザー別フィード設定取得
    start_time=$(date +%s%N)
    query_main "SELECT COUNT(*) FROM user_feed_settings WHERE user_id = '$DEFAULT_USER_ID';" >/dev/null
    end_time=$(date +%s%N)
    duration=$(((end_time - start_time) / 1000000))
    
    if [[ "$duration" -lt 500 ]]; then  # 500ms未満
        test_pass "User feed settings query performance: ${duration}ms"
    else
        test_warn "User feed settings query slow: ${duration}ms"
    fi
    
    # 3. セッション検証クエリ
    start_time=$(date +%s%N)
    query_auth "SELECT COUNT(*) FROM user_sessions WHERE user_id = '$DEFAULT_USER_ID' AND active = true;" >/dev/null
    end_time=$(date +%s%N)
    duration=$(((end_time - start_time) / 1000000))
    
    if [[ "$duration" -lt 100 ]]; then  # 100ms未満
        test_pass "Session validation query performance: ${duration}ms"
    else
        test_warn "Session validation query slow: ${duration}ms"
    fi
}

# 関数テスト
verify_functions() {
    log "Verifying database functions..."
    
    # Auth-Postgres 関数
    local auth_functions=(
        "get_tenant_by_slug"
        "is_tenant_active"
        "get_user_by_kratos_id"
        "is_user_active"
        "log_audit_event"
        "get_user_preference"
        "create_csrf_token"
        "validate_csrf_token"
    )
    
    for func in "${auth_functions[@]}"; do
        if query_auth "SELECT 1 FROM pg_proc WHERE proname = '$func';" >/dev/null 2>&1; then
            test_pass "Auth function exists: $func"
        else
            test_fail "Auth function missing: $func"
        fi
    done
    
    # Main-Postgres 関数
    local main_functions=(
        "get_user_feed_settings"
        "get_user_tags"
        "get_user_folder_hierarchy"
        "get_user_stats"
    )
    
    for func in "${main_functions[@]}"; do
        if query_main "SELECT 1 FROM pg_proc WHERE proname = '$func';" >/dev/null 2>&1; then
            test_pass "Main function exists: $func"
        else
            test_fail "Main function missing: $func"
        fi
    done
}

# 統計情報チェック
verify_statistics() {
    log "Checking database statistics..."
    
    # テーブルサイズ情報
    local tables=("read_status" "favorite_feeds" "feed_links" "user_feed_settings" "user_tags")
    
    for table in "${tables[@]}"; do
        local size
        size=$(query_main "SELECT pg_size_pretty(pg_total_relation_size('$table'));")
        log "Table $table size: $size"
    done
    
    # Auth データベースサイズ
    local auth_tables=("tenants" "users" "user_sessions" "audit_logs")
    
    for table in "${auth_tables[@]}"; do
        local size
        size=$(query_auth "SELECT pg_size_pretty(pg_total_relation_size('$table'));")
        log "Auth table $table size: $size"
    done
    
    test_pass "Database statistics collected"
}

# 移行データサンプリングチェック
verify_data_samples() {
    log "Verifying data samples..."
    
    # デフォルトユーザーのデータ存在確認
    local read_status_count
    read_status_count=$(query_main "SELECT COUNT(*) FROM read_status WHERE user_id = '$DEFAULT_USER_ID';")
    
    if [[ "$read_status_count" -gt 0 ]]; then
        test_pass "Default user has read status records: $read_status_count"
    else
        test_warn "Default user has no read status records"
    fi
    
    # デフォルトデータの確認
    local default_tags_count
    default_tags_count=$(query_main "SELECT COUNT(*) FROM user_tags WHERE user_id = '$DEFAULT_USER_ID';")
    
    if [[ "$default_tags_count" -gt 0 ]]; then
        test_pass "Default user has tags: $default_tags_count"
    else
        test_warn "Default user has no tags"
    fi
    
    local default_folders_count
    default_folders_count=$(query_main "SELECT COUNT(*) FROM user_folders WHERE user_id = '$DEFAULT_USER_ID';")
    
    if [[ "$default_folders_count" -gt 0 ]]; then
        test_pass "Default user has folders: $default_folders_count"
    else
        test_warn "Default user has no folders"
    fi
    
    # テナント・ユーザー設定確認
    local tenant_settings
    tenant_settings=$(query_auth "SELECT settings FROM tenants WHERE id = '$DEFAULT_USER_ID';")
    
    if [[ -n "$tenant_settings" ]]; then
        test_pass "Default tenant has settings configured"
    else
        test_fail "Default tenant settings missing"
    fi
}

# 総合レポート生成
generate_report() {
    log "Generating verification report..."
    
    local total_tests=$((TESTS_PASSED + TESTS_FAILED + TESTS_WARNINGS))
    local success_rate
    
    if [[ "$total_tests" -gt 0 ]]; then
        success_rate=$(((TESTS_PASSED * 100) / total_tests))
    else
        success_rate=0
    fi
    
    cat << EOF | tee -a "$LOG_FILE"

=== MIGRATION VERIFICATION REPORT ===
Generated: $(date)

Test Results:
  ✅ Passed:   $TESTS_PASSED
  ❌ Failed:   $TESTS_FAILED
  ⚠️  Warnings: $TESTS_WARNINGS
  📊 Total:    $total_tests
  🎯 Success Rate: $success_rate%

Database Status:
  📦 Main-Postgres: $(query_main "SELECT version();" | head -1 || echo "Connection failed")
  🔐 Auth-Postgres: $(query_auth "SELECT version();" | head -1 || echo "Connection failed")

Migration Status:
$(if [[ "$TESTS_FAILED" -eq 0 ]]; then
    echo "  ✅ MIGRATION SUCCESSFUL"
    echo "     All critical tests passed. Database is ready for use."
elif [[ "$TESTS_FAILED" -lt 5 ]]; then
    echo "  ⚠️  MIGRATION COMPLETED WITH WARNINGS"
    echo "     Some non-critical issues found. Review failed tests."
else
    echo "  ❌ MIGRATION FAILED"
    echo "     Critical issues found. Do not proceed to production."
fi)

Recommendations:
$(if [[ "$TESTS_FAILED" -gt 0 ]]; then
    echo "  - Review failed tests in the log file"
    echo "  - Consider running rollback if critical issues exist"
fi)
$(if [[ "$TESTS_WARNINGS" -gt 0 ]]; then
    echo "  - Review warnings for potential performance issues"
    echo "  - Monitor database performance after deployment"
fi)
$(if [[ "$TESTS_FAILED" -eq 0 ]]; then
    echo "  - Update application connection strings"
    echo "  - Deploy updated application code"
    echo "  - Monitor application logs for integration issues"
fi)

Log File: $LOG_FILE
EOF
}

# メイン実行
main() {
    log "=== Migration Verification Started ==="
    log "Main DB: $MAIN_DB_HOST:$MAIN_DB_PORT/$MAIN_DB_NAME"
    log "Auth DB: $AUTH_DB_HOST:$AUTH_DB_PORT/$AUTH_DB_NAME"
    log "Log file: $LOG_FILE"
    
    # 検証テスト実行
    check_connections
    verify_schema_structure
    verify_data_integrity
    verify_indexes
    verify_constraints
    verify_functions
    verify_performance
    verify_statistics
    verify_data_samples
    
    # レポート生成
    generate_report
    
    log "=== Migration Verification Completed ==="
    log "Total time: $SECONDS seconds"
    
    # 終了コード決定
    if [[ "$TESTS_FAILED" -eq 0 ]]; then
        echo ""
        echo "✅ Migration verification completed successfully!"
        echo "📊 Test Results: $TESTS_PASSED passed, $TESTS_WARNINGS warnings"
        echo "📝 Full report: $LOG_FILE"
        exit 0
    else
        echo ""
        echo "❌ Migration verification failed!"
        echo "📊 Test Results: $TESTS_PASSED passed, $TESTS_FAILED failed, $TESTS_WARNINGS warnings"
        echo "📝 Full report: $LOG_FILE"
        exit 1
    fi
}

# エラーハンドリング
trap 'error "Verification interrupted"' INT TERM

# 実行
main "$@"