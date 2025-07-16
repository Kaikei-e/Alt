-- Drop csrf_tokens table and related objects
DROP FUNCTION IF EXISTS validate_csrf_token(VARCHAR(255), VARCHAR(255));
DROP FUNCTION IF EXISTS cleanup_expired_csrf_tokens();
DROP TABLE IF EXISTS csrf_tokens CASCADE;