-- Revoke search indexer user permissions

REVOKE SELECT ON articles FROM search_indexer_user;
REVOKE SELECT ON tags FROM search_indexer_user;
REVOKE SELECT ON article_tags FROM search_indexer_user;
REVOKE USAGE ON SCHEMA public FROM search_indexer_user;
REVOKE CONNECT ON DATABASE alt FROM search_indexer_user;