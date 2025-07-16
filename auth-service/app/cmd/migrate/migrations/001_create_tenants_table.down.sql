-- Drop tenants table and related objects
DROP TRIGGER IF EXISTS update_tenants_updated_at ON tenants;
DROP FUNCTION IF EXISTS update_updated_at_column();
DROP TABLE IF EXISTS tenants CASCADE;