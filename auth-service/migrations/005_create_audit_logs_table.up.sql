-- Create audit_logs table for security auditing (partitioned by month)
CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_id VARCHAR(255),
    details JSONB DEFAULT '{}',
    ip_address INET,
    user_agent TEXT,
    session_id VARCHAR(255),
    success BOOLEAN NOT NULL DEFAULT true,
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    -- Partition key must be included in primary key
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Create indexes for the parent table
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_id ON audit_logs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource_type ON audit_logs(resource_type);
CREATE INDEX IF NOT EXISTS idx_audit_logs_success ON audit_logs(success);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at);

-- Create composite indexes for common queries
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_created ON audit_logs(tenant_id, created_at);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_created ON audit_logs(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action_created ON audit_logs(action, created_at);

-- Create initial partitions for current and next month
DO $$
DECLARE
    current_month DATE := DATE_TRUNC('month', CURRENT_DATE);
    next_month DATE := DATE_TRUNC('month', CURRENT_DATE + INTERVAL '1 month');
    partition_name TEXT;
BEGIN
    -- Current month partition
    partition_name := 'audit_logs_' || TO_CHAR(current_month, 'YYYY_MM');
    EXECUTE format('CREATE TABLE IF NOT EXISTS %I PARTITION OF audit_logs
        FOR VALUES FROM (%L) TO (%L)', 
        partition_name, current_month, next_month);
        
    -- Next month partition
    partition_name := 'audit_logs_' || TO_CHAR(next_month, 'YYYY_MM');
    EXECUTE format('CREATE TABLE IF NOT EXISTS %I PARTITION OF audit_logs
        FOR VALUES FROM (%L) TO (%L)', 
        partition_name, next_month, next_month + INTERVAL '1 month');
END $$;

-- Create function to automatically create monthly partitions
CREATE OR REPLACE FUNCTION create_audit_logs_partition(partition_date DATE)
RETURNS VOID AS $$
DECLARE
    partition_name TEXT;
    start_date DATE;
    end_date DATE;
BEGIN
    start_date := DATE_TRUNC('month', partition_date);
    end_date := start_date + INTERVAL '1 month';
    partition_name := 'audit_logs_' || TO_CHAR(start_date, 'YYYY_MM');
    
    EXECUTE format('CREATE TABLE IF NOT EXISTS %I PARTITION OF audit_logs
        FOR VALUES FROM (%L) TO (%L)', 
        partition_name, start_date, end_date);
        
    -- Create indexes on the partition
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I ON %I(tenant_id, created_at)', 
        'idx_' || partition_name || '_tenant_created', partition_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I ON %I(user_id, created_at)', 
        'idx_' || partition_name || '_user_created', partition_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I ON %I(action)', 
        'idx_' || partition_name || '_action', partition_name);
END;
$$ LANGUAGE plpgsql;

-- Create function to clean up old audit log partitions
CREATE OR REPLACE FUNCTION cleanup_old_audit_partitions(months_to_keep INTEGER DEFAULT 12)
RETURNS INTEGER AS $$
DECLARE
    cutoff_date DATE;
    partition_name TEXT;
    dropped_count INTEGER := 0;
BEGIN
    cutoff_date := DATE_TRUNC('month', CURRENT_DATE - (months_to_keep || ' months')::INTERVAL);
    
    FOR partition_name IN 
        SELECT tablename 
        FROM pg_tables 
        WHERE tablename LIKE 'audit_logs_%' 
        AND schemaname = 'public'
        AND TO_DATE(RIGHT(tablename, 7), 'YYYY_MM') < cutoff_date
    LOOP
        EXECUTE format('DROP TABLE IF EXISTS %I CASCADE', partition_name);
        dropped_count := dropped_count + 1;
    END LOOP;
    
    RETURN dropped_count;
END;
$$ LANGUAGE plpgsql;

-- Create function to log audit events
CREATE OR REPLACE FUNCTION log_audit_event(
    p_tenant_id UUID,
    p_user_id UUID,
    p_action VARCHAR(100),
    p_resource_type VARCHAR(50),
    p_resource_id VARCHAR(255) DEFAULT NULL,
    p_details JSONB DEFAULT '{}',
    p_ip_address INET DEFAULT NULL,
    p_user_agent TEXT DEFAULT NULL,
    p_session_id VARCHAR(255) DEFAULT NULL,
    p_success BOOLEAN DEFAULT true,
    p_error_message TEXT DEFAULT NULL
)
RETURNS UUID AS $$
DECLARE
    audit_id UUID;
BEGIN
    INSERT INTO audit_logs (
        tenant_id, user_id, action, resource_type, resource_id,
        details, ip_address, user_agent, session_id, success, error_message
    ) VALUES (
        p_tenant_id, p_user_id, p_action, p_resource_type, p_resource_id,
        p_details, p_ip_address, p_user_agent, p_session_id, p_success, p_error_message
    ) RETURNING id INTO audit_id;
    
    RETURN audit_id;
END;
$$ LANGUAGE plpgsql;