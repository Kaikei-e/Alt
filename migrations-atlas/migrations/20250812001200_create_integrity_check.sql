-- Migration: create integrity check procedure
-- File: migrations-atlas/migrations/20250812001200_create_integrity_check.sql
-- Author: Claude Code
-- Date: 2025-08-12
-- Purpose: Create functions to check data integrity between main database and auth service

-- Function to check user references in main database tables
CREATE OR REPLACE FUNCTION check_user_references()
RETURNS TABLE(table_name text, invalid_count bigint, details text) AS $$
BEGIN
    -- Check read_status table
    RETURN QUERY
    SELECT 
        'read_status'::text,
        COUNT(*),
        'Users referenced in read_status that may not exist in auth service'::text
    FROM read_status rs
    WHERE rs.user_id != '00000000-0000-0000-0000-000000000001'::uuid;
    
    -- Check favorite_feeds table
    RETURN QUERY
    SELECT 
        'favorite_feeds'::text,
        COUNT(*),
        'Users referenced in favorite_feeds that may not exist in auth service'::text
    FROM favorite_feeds ff
    WHERE ff.user_id != '00000000-0000-0000-0000-000000000001'::uuid;
    
    -- Check for orphaned records with legacy user ID
    RETURN QUERY
    SELECT 
        'legacy_read_status'::text,
        COUNT(*),
        'Records using legacy user ID in read_status'::text
    FROM read_status rs
    WHERE rs.user_id = '00000000-0000-0000-0000-000000000001'::uuid;
    
    RETURN QUERY
    SELECT 
        'legacy_favorite_feeds'::text,
        COUNT(*),
        'Records using legacy user ID in favorite_feeds'::text
    FROM favorite_feeds ff
    WHERE ff.user_id = '00000000-0000-0000-0000-000000000001'::uuid;
END;
$$ LANGUAGE plpgsql;

-- Function to check data consistency within main database
CREATE OR REPLACE FUNCTION check_data_consistency()
RETURNS TABLE(check_name text, status text, details text) AS $$
BEGIN
    -- Check read_status referential integrity
    RETURN QUERY
    SELECT 
        'read_status_feed_references'::text,
        CASE 
            WHEN COUNT(*) = 0 THEN 'PASS'::text
            ELSE 'FAIL'::text
        END,
        'Read status entries with invalid feed references: ' || COUNT(*)::text
    FROM read_status rs
    LEFT JOIN feeds f ON rs.feed_id = f.id
    WHERE f.id IS NULL;
    
    -- Check favorite_feeds referential integrity
    RETURN QUERY
    SELECT 
        'favorite_feeds_feed_references'::text,
        CASE 
            WHEN COUNT(*) = 0 THEN 'PASS'::text
            ELSE 'FAIL'::text
        END,
        'Favorite feeds entries with invalid feed references: ' || COUNT(*)::text
    FROM favorite_feeds ff
    LEFT JOIN feeds f ON ff.feed_id = f.id
    WHERE f.id IS NULL;
    
    -- Check for duplicate user-feed combinations in read_status
    RETURN QUERY
    SELECT 
        'read_status_duplicates'::text,
        CASE 
            WHEN COUNT(*) = 0 THEN 'PASS'::text
            ELSE 'FAIL'::text
        END,
        'Duplicate user-feed combinations in read_status: ' || COUNT(*)::text
    FROM (
        SELECT user_id, feed_id, COUNT(*) as cnt
        FROM read_status
        GROUP BY user_id, feed_id
        HAVING COUNT(*) > 1
    ) duplicates;
    
    -- Check for duplicate user-feed combinations in favorite_feeds
    RETURN QUERY
    SELECT 
        'favorite_feeds_duplicates'::text,
        CASE 
            WHEN COUNT(*) = 0 THEN 'PASS'::text
            ELSE 'FAIL'::text
        END,
        'Duplicate user-feed combinations in favorite_feeds: ' || COUNT(*)::text
    FROM (
        SELECT user_id, feed_id, COUNT(*) as cnt
        FROM favorite_feeds
        GROUP BY user_id, feed_id
        HAVING COUNT(*) > 1
    ) duplicates;
END;
$$ LANGUAGE plpgsql;

-- Function to get migration status summary
CREATE OR REPLACE FUNCTION get_migration_status()
RETURNS TABLE(metric_name text, value bigint, percentage numeric) AS $$
BEGIN
    -- Total records using legacy user ID
    RETURN QUERY
    SELECT 
        'total_legacy_records'::text,
        (SELECT COUNT(*) FROM read_status WHERE user_id = '00000000-0000-0000-0000-000000000001'::uuid) +
        (SELECT COUNT(*) FROM favorite_feeds WHERE user_id = '00000000-0000-0000-0000-000000000001'::uuid),
        100.0::numeric;
    
    -- read_status statistics
    RETURN QUERY
    SELECT 
        'read_status_total'::text,
        COUNT(*),
        100.0::numeric
    FROM read_status;
    
    RETURN QUERY
    SELECT 
        'read_status_legacy'::text,
        COUNT(*),
        ROUND(COUNT(*)::numeric * 100.0 / NULLIF((SELECT COUNT(*) FROM read_status), 0), 2)
    FROM read_status
    WHERE user_id = '00000000-0000-0000-0000-000000000001'::uuid;
    
    -- favorite_feeds statistics
    RETURN QUERY
    SELECT 
        'favorite_feeds_total'::text,
        COUNT(*),
        100.0::numeric
    FROM favorite_feeds;
    
    RETURN QUERY
    SELECT 
        'favorite_feeds_legacy'::text,
        COUNT(*),
        ROUND(COUNT(*)::numeric * 100.0 / NULLIF((SELECT COUNT(*) FROM favorite_feeds), 0), 2)
    FROM favorite_feeds
    WHERE user_id = '00000000-0000-0000-0000-000000000001'::uuid;
END;
$$ LANGUAGE plpgsql;

-- Create a view for easy monitoring
CREATE OR REPLACE VIEW migration_dashboard AS
SELECT 
    'Data Integrity Check' as category,
    check_name,
    status,
    details
FROM check_data_consistency()
UNION ALL
SELECT 
    'Migration Status' as category,
    metric_name as check_name,
    value::text as status,
    percentage::text || '%' as details
FROM get_migration_status();

-- Schedule regular checks (requires pg_cron extension, commented for now)
-- SELECT cron.schedule('integrity-check', '0 2 * * *', 'SELECT check_user_references();');
-- SELECT cron.schedule('consistency-check', '0 3 * * *', 'SELECT check_data_consistency();');