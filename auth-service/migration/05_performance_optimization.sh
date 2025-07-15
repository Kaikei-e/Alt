#!/bin/bash

# ==========================================
# 05_performance_optimization.sh
# データベースパフォーマンス最適化スクリプト
# ==========================================

set -e  # エラー時に停止
set -u  # 未定義変数使用時に停止

# 設定値
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_FILE="${SCRIPT_DIR}/performance_optimization_$(date +%Y%m%d_%H%M%S).log"

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

# 最適化レベル
OPTIMIZATION_LEVEL="${OPTIMIZATION_LEVEL:-standard}"  # light, standard, aggressive

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

success() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') [PASS] $*" | tee -a "$LOG_FILE"
}

# PostgreSQL クエリ実行
execute_sql() {
    local db="$1"
    local sql="$2"
    local description="$3"
    
    log "Executing: $description"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log "[DRY RUN] Would execute: $sql"
        return 0
    fi
    
    if [[ "$db" == "main" ]]; then
        if PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "$sql" >> "$LOG_FILE" 2>&1; then
            success "$description completed"
            return 0
        else
            warning "$description failed"
            return 1
        fi
    elif [[ "$db" == "auth" ]]; then
        if PGPASSWORD="$AUTH_DB_PASSWORD" psql -h "$AUTH_DB_HOST" -p "$AUTH_DB_PORT" -U "$AUTH_DB_USER" -d "$AUTH_DB_NAME" -c "$sql" >> "$LOG_FILE" 2>&1; then
            success "$description completed"
            return 0
        else
            warning "$description failed"
            return 1
        fi
    fi
}

# データベース統計情報更新
update_statistics() {
    log "Updating database statistics..."
    
    # Main-Postgres
    execute_sql "main" "ANALYZE;" "Main database statistics update"
    
    # Auth-Postgres
    execute_sql "auth" "ANALYZE;" "Auth database statistics update"
    
    # テーブル別統計更新
    local main_tables=("read_status" "favorite_feeds" "feed_links" "user_feed_settings" "user_tags" "user_article_tags")
    
    for table in "${main_tables[@]}"; do
        execute_sql "main" "ANALYZE $table;" "Statistics update for $table"
    done
    
    local auth_tables=("tenants" "users" "user_sessions" "csrf_tokens" "audit_logs" "user_preferences")
    
    for table in "${auth_tables[@]}"; do
        execute_sql "auth" "ANALYZE $table;" "Auth statistics update for $table"
    done
}

# パーティション最適化
optimize_partitions() {
    log "Optimizing partitioned tables..."
    
    # 監査ログの追加パーティション作成（次の6ヶ月分）
    local sql="
    DO \$\$
    DECLARE
        current_month DATE;
        i INTEGER;
    BEGIN
        current_month := date_trunc('month', CURRENT_DATE);
        
        FOR i IN 1..6 LOOP
            current_month := current_month + INTERVAL '1 month';
            BEGIN
                PERFORM create_monthly_partition('audit_logs', current_month);
            EXCEPTION WHEN duplicate_table THEN
                CONTINUE;
            END;
        END LOOP;
    END \$\$;
    "
    
    execute_sql "auth" "$sql" "Create additional audit log partitions"
    
    # 古いパーティションの統計更新
    sql="
    DO \$\$
    DECLARE
        partition_record RECORD;
    BEGIN
        FOR partition_record IN
            SELECT schemaname, tablename
            FROM pg_tables
            WHERE tablename LIKE 'audit_logs_y%'
                AND schemaname = 'public'
        LOOP
            EXECUTE format('ANALYZE %I', partition_record.tablename);
        END LOOP;
    END \$\$;
    "
    
    execute_sql "auth" "$sql" "Update partition statistics"
}

# インデックス最適化
optimize_indexes() {
    log "Optimizing indexes..."
    
    # 高頻度クエリ用の複合インデックス（Main DB）
    local main_indexes=(
        "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_read_status_user_unread ON read_status(user_id, feed_id) WHERE is_read = false;"
        "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_articles_feed_pubdate_desc ON articles(feed_id, pub_date DESC);"
        "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_user_feed_settings_priority_desc ON user_feed_settings(user_id, priority DESC, notification_enabled);"
        "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_user_article_tags_user_tag ON user_article_tags(user_id, tag_id);"
        "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_user_folders_user_parent ON user_folders(user_id, parent_id, sort_order);"
    )
    
    for index_sql in "${main_indexes[@]}"; do
        execute_sql "main" "$index_sql" "Create optimized main index"
    done
    
    # Auth DB の最適化インデックス
    local auth_indexes=(
        "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_user_sessions_user_active_activity ON user_sessions(user_id, active, last_activity_at DESC) WHERE active = true;"
        "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_csrf_tokens_session_expires ON csrf_tokens(session_id, expires_at) WHERE expires_at > CURRENT_TIMESTAMP;"
        "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_logs_tenant_action_created ON audit_logs(tenant_id, action, created_at DESC);"
        "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_user_preferences_user_category_key ON user_preferences(user_id, category, key);"
    )
    
    for index_sql in "${auth_indexes[@]}"; do
        execute_sql "auth" "$index_sql" "Create optimized auth index"
    done
    
    # アグレッシブ最適化（大量データ想定）
    if [[ "$OPTIMIZATION_LEVEL" == "aggressive" ]]; then
        log "Applying aggressive index optimization..."
        
        local aggressive_indexes=(
            "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_articles_content_gin ON articles USING GIN(to_tsvector('english', title || ' ' || COALESCE(description, '')));"
            "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_feeds_url_hash ON feeds(md5(url));"
            "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_user_sessions_ip_activity ON user_sessions(ip_address, last_activity_at DESC);"
        )
        
        for index_sql in "${aggressive_indexes[@]}"; do
            execute_sql "main" "$index_sql" "Create aggressive optimization index"
        done
    fi
}

# バキューム・リオーガニゼーション
vacuum_optimization() {
    log "Performing vacuum optimization..."
    
    # 通常のVACUUM ANALYZE
    if [[ "$OPTIMIZATION_LEVEL" == "light" ]]; then
        execute_sql "main" "VACUUM ANALYZE;" "Light vacuum on main database"
        execute_sql "auth" "VACUUM ANALYZE;" "Light vacuum on auth database"
        return
    fi
    
    # より詳細なVACUUM
    local main_tables=("read_status" "favorite_feeds" "feed_links" "articles" "feeds")
    
    for table in "${main_tables[@]}"; do
        if [[ "$OPTIMIZATION_LEVEL" == "aggressive" ]]; then
            execute_sql "main" "VACUUM FULL ANALYZE $table;" "Full vacuum on $table"
        else
            execute_sql "main" "VACUUM ANALYZE $table;" "Vacuum analyze on $table"
        fi
    done
    
    local auth_tables=("users" "user_sessions" "csrf_tokens" "audit_logs")
    
    for table in "${auth_tables[@]}"; do
        if [[ "$OPTIMIZATION_LEVEL" == "aggressive" ]]; then
            execute_sql "auth" "VACUUM FULL ANALYZE $table;" "Full vacuum on auth $table"
        else
            execute_sql "auth" "VACUUM ANALYZE $table;" "Vacuum analyze on auth $table"
        fi
    done
}

# 設定最適化の提案
suggest_configuration() {
    log "Generating configuration recommendations..."
    
    cat << EOF | tee -a "$LOG_FILE"

=== PostgreSQL Configuration Recommendations ===

Based on workload analysis, consider these postgresql.conf settings:

# Memory Settings
shared_buffers = 256MB                    # 25% of RAM for dedicated server
effective_cache_size = 1GB                # 75% of RAM
work_mem = 4MB                            # Per connection sort/hash memory
maintenance_work_mem = 64MB               # For VACUUM, INDEX operations

# Checkpoint Settings
checkpoint_completion_target = 0.9        # Spread checkpoints over 90% of interval
wal_buffers = 16MB                        # WAL buffer size
checkpoint_timeout = 10min                # Checkpoint frequency

# Autovacuum Settings
autovacuum = on
autovacuum_naptime = 1min                 # Frequency of autovacuum runs
autovacuum_vacuum_threshold = 50          # Min number of updated tuples
autovacuum_analyze_threshold = 50         # Min number for analyze
autovacuum_vacuum_scale_factor = 0.2      # Fraction of table size
autovacuum_analyze_scale_factor = 0.1     # Fraction for analyze
autovacuum_vacuum_cost_delay = 20ms       # Delay between vacuum cycles

# Query Planning
random_page_cost = 1.1                    # For SSD storage
effective_io_concurrency = 200            # For SSD storage
default_statistics_target = 100           # Statistics collection detail

# Logging and Monitoring
log_min_duration_statement = 1000         # Log queries taking > 1 second
log_checkpoints = on
log_connections = on
log_disconnections = on
log_lock_waits = on
log_temp_files = 0

# Connection Settings
max_connections = 100                     # Adjust based on application needs
shared_preload_libraries = 'pg_stat_statements'

# For Auth Database (lighter workload)
# Consider reducing some settings for auth-postgres:
# shared_buffers = 128MB
# work_mem = 2MB
# maintenance_work_mem = 32MB

EOF

    # ワークロード別推奨事項
    case "$OPTIMIZATION_LEVEL" in
        "light")
            cat << EOF | tee -a "$LOG_FILE"

=== Light Optimization Recommendations ===
- Current settings are conservative and suitable for small-medium workloads
- Monitor pg_stat_user_tables for vacuum/analyze frequency
- Consider increasing shared_buffers if you have dedicated database server

EOF
            ;;
        "standard")
            cat << EOF | tee -a "$LOG_FILE"

=== Standard Optimization Recommendations ===
- Enable pg_stat_statements for query performance monitoring
- Set up regular VACUUM and ANALYZE jobs for large tables
- Monitor index usage with pg_stat_user_indexes
- Consider partitioning audit_logs if it grows > 10M records

EOF
            ;;
        "aggressive")
            cat << EOF | tee -a "$LOG_FILE"

=== Aggressive Optimization Recommendations ===
- Consider connection pooling (PgBouncer) for high-concurrency workloads
- Implement read replicas for reporting queries
- Set up monitoring with pg_stat_statements and pgBadger
- Consider pg_repack for table reorganization without locks
- Implement automated partition management for audit_logs

EOF
            ;;
    esac
}

# クエリパフォーマンステスト
test_query_performance() {
    log "Testing query performance..."
    
    # テストクエリとベンチマーク
    local test_queries=(
        "SELECT COUNT(*) FROM read_status WHERE user_id = '00000000-0000-0000-0000-000000000001' AND is_read = false"
        "SELECT COUNT(*) FROM user_sessions WHERE user_id = '00000000-0000-0000-0000-000000000001' AND active = true"
        "SELECT COUNT(*) FROM audit_logs WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND created_at >= CURRENT_DATE - INTERVAL '7 days'"
    )
    
    local db_targets=("main" "auth" "auth")
    local query_names=("Read Status Lookup" "Active Sessions" "Recent Audit Logs")
    
    for i in "${!test_queries[@]}"; do
        local query="${test_queries[$i]}"
        local db="${db_targets[$i]}"
        local name="${query_names[$i]}"
        
        log "Testing: $name"
        
        if [[ "$DRY_RUN" != "true" ]]; then
            local start_time
            local end_time
            local duration
            
            start_time=$(date +%s%N)
            
            if [[ "$db" == "main" ]]; then
                PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "$query" >/dev/null 2>&1
            else
                PGPASSWORD="$AUTH_DB_PASSWORD" psql -h "$AUTH_DB_HOST" -p "$AUTH_DB_PORT" -U "$AUTH_DB_USER" -d "$AUTH_DB_NAME" -c "$query" >/dev/null 2>&1
            fi
            
            end_time=$(date +%s%N)
            duration=$(((end_time - start_time) / 1000000))  # Convert to milliseconds
            
            if [[ "$duration" -lt 100 ]]; then
                success "$name: ${duration}ms (Excellent)"
            elif [[ "$duration" -lt 500 ]]; then
                log "$name: ${duration}ms (Good)"
            elif [[ "$duration" -lt 1000 ]]; then
                warning "$name: ${duration}ms (Acceptable)"
            else
                warning "$name: ${duration}ms (Needs optimization)"
            fi
        else
            log "[DRY RUN] Would test: $name"
        fi
    done
}

# インデックス使用状況分析
analyze_index_usage() {
    log "Analyzing index usage..."
    
    # メインデータベースのインデックス使用状況
    local sql="
    SELECT 
        schemaname,
        tablename,
        indexname,
        idx_scan,
        idx_tup_read,
        idx_tup_fetch
    FROM pg_stat_user_indexes 
    WHERE schemaname = 'public'
    ORDER BY idx_scan DESC;
    "
    
    if [[ "$DRY_RUN" != "true" ]]; then
        log "Main database index usage:"
        PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "$sql" >> "$LOG_FILE" 2>&1
        
        log "Auth database index usage:"
        PGPASSWORD="$AUTH_DB_PASSWORD" psql -h "$AUTH_DB_HOST" -p "$AUTH_DB_PORT" -U "$AUTH_DB_USER" -d "$AUTH_DB_NAME" -c "$sql" >> "$LOG_FILE" 2>&1
    else
        log "[DRY RUN] Would analyze index usage"
    fi
    
    # 未使用インデックスの検出
    sql="
    SELECT 
        schemaname,
        tablename,
        indexname,
        pg_size_pretty(pg_relation_size(indexrelid)) as size
    FROM pg_stat_user_indexes s
    JOIN pg_index i ON s.indexrelid = i.indexrelid
    WHERE idx_scan = 0
        AND NOT i.indisunique
        AND NOT i.indisprimary
        AND schemaname = 'public'
    ORDER BY pg_relation_size(indexrelid) DESC;
    "
    
    if [[ "$DRY_RUN" != "true" ]]; then
        log "Unused indexes in main database:"
        PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "$sql" >> "$LOG_FILE" 2>&1
        
        log "Unused indexes in auth database:"
        PGPASSWORD="$AUTH_DB_PASSWORD" psql -h "$AUTH_DB_HOST" -p "$AUTH_DB_PORT" -U "$AUTH_DB_USER" -d "$AUTH_DB_NAME" -c "$sql" >> "$LOG_FILE" 2>&1
    else
        log "[DRY RUN] Would check for unused indexes"
    fi
}

# データベースサイズ分析
analyze_database_size() {
    log "Analyzing database sizes..."
    
    local sql="
    SELECT 
        tablename,
        pg_size_pretty(pg_total_relation_size(tablename::regclass)) as size,
        pg_size_pretty(pg_relation_size(tablename::regclass)) as table_size,
        pg_size_pretty(pg_total_relation_size(tablename::regclass) - pg_relation_size(tablename::regclass)) as index_size
    FROM pg_tables 
    WHERE schemaname = 'public'
    ORDER BY pg_total_relation_size(tablename::regclass) DESC;
    "
    
    if [[ "$DRY_RUN" != "true" ]]; then
        log "Main database table sizes:"
        PGPASSWORD="$MAIN_DB_PASSWORD" psql -h "$MAIN_DB_HOST" -p "$MAIN_DB_PORT" -U "$MAIN_DB_USER" -d "$MAIN_DB_NAME" -c "$sql" >> "$LOG_FILE" 2>&1
        
        log "Auth database table sizes:"
        PGPASSWORD="$AUTH_DB_PASSWORD" psql -h "$AUTH_DB_HOST" -p "$AUTH_DB_PORT" -U "$AUTH_DB_USER" -d "$AUTH_DB_NAME" -c "$sql" >> "$LOG_FILE" 2>&1
    else
        log "[DRY RUN] Would analyze database sizes"
    fi
}

# メイン実行
main() {
    log "=== Database Performance Optimization Started ==="
    log "Optimization Level: $OPTIMIZATION_LEVEL"
    log "Dry Run: $DRY_RUN"
    log "Log file: $LOG_FILE"
    
    # 最適化実行
    log "Phase 1: Statistics and Analysis"
    update_statistics
    analyze_database_size
    analyze_index_usage
    
    log "Phase 2: Index Optimization"
    optimize_indexes
    
    log "Phase 3: Partition Optimization"
    optimize_partitions
    
    log "Phase 4: Vacuum and Maintenance"
    vacuum_optimization
    
    log "Phase 5: Performance Testing"
    test_query_performance
    
    log "Phase 6: Configuration Recommendations"
    suggest_configuration
    
    log "=== Database Performance Optimization Completed ==="
    log "Total time: $SECONDS seconds"
    
    echo ""
    if [[ "$DRY_RUN" == "true" ]]; then
        echo "🧪 Performance optimization dry run completed!"
        echo "   Review the recommendations in the log file."
    else
        echo "✅ Performance optimization completed successfully!"
        echo "📈 Optimization level: $OPTIMIZATION_LEVEL"
    fi
    echo "📝 Log file: $LOG_FILE"
    echo ""
    echo "Next steps:"
    echo "1. Review configuration recommendations"
    echo "2. Monitor query performance over time"
    echo "3. Set up automated maintenance jobs"
    echo "4. Consider implementing monitoring tools"
}

# 使用方法表示
show_usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Optimize database performance for the Alt RSS reader system.

Options:
  -l, --level LEVEL     Optimization level: light, standard, aggressive (default: standard)
  -d, --dry-run         Show what would be done without making changes
  -h, --help           Show this help message

Optimization Levels:
  light                Basic statistics update and light vacuum
  standard             Comprehensive optimization with new indexes and vacuum
  aggressive           Full optimization including VACUUM FULL and advanced indexes

Environment Variables:
  MAIN_DB_*            Main database connection settings
  AUTH_DB_*            Auth database connection settings

Examples:
  # Dry run with standard optimization
  DRY_RUN=true $0 --level standard
  
  # Light optimization for production
  $0 --level light
  
  # Aggressive optimization during maintenance window
  $0 --level aggressive
EOF
}

# コマンドライン引数解析
parse_arguments() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -l|--level)
                OPTIMIZATION_LEVEL="$2"
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
    
    if [[ ! "$OPTIMIZATION_LEVEL" =~ ^(light|standard|aggressive)$ ]]; then
        error "Invalid optimization level: $OPTIMIZATION_LEVEL"
    fi
}

# エラーハンドリング
trap 'error "Performance optimization interrupted"' INT TERM

# 実行
parse_arguments "$@"
main