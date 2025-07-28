-- Remove sslinfo extension
-- INCIDENT 63 ROLLBACK: Remove ssl_is_used() function support

DROP EXTENSION IF EXISTS sslinfo CASCADE;