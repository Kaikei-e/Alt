-- Install sslinfo extension for SSL status monitoring
-- INCIDENT 63 FIX: Resolve ssl_is_used() function not found error
-- Reference: https://www.postgresql.org/docs/current/sslinfo.html

CREATE EXTENSION IF NOT EXISTS sslinfo;

-- Verify installation by testing ssl_is_used function
-- This will return boolean indicating if current connection uses SSL
SELECT ssl_is_used() AS ssl_connection_status;