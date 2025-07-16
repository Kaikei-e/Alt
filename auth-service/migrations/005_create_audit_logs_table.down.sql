-- Drop audit_logs table and related objects
DROP FUNCTION IF EXISTS log_audit_event(UUID, UUID, VARCHAR(100), VARCHAR(50), VARCHAR(255), JSONB, INET, TEXT, VARCHAR(255), BOOLEAN, TEXT);
DROP FUNCTION IF EXISTS cleanup_old_audit_partitions(INTEGER);
DROP FUNCTION IF EXISTS create_audit_logs_partition(DATE);

-- Drop all audit log partitions
DO $$
DECLARE
    partition_name TEXT;
BEGIN
    FOR partition_name IN 
        SELECT tablename 
        FROM pg_tables 
        WHERE tablename LIKE 'audit_logs_%' 
        AND schemaname = 'public'
    LOOP
        EXECUTE format('DROP TABLE IF EXISTS %I CASCADE', partition_name);
    END LOOP;
END $$;

-- Drop parent table
DROP TABLE IF EXISTS audit_logs CASCADE;