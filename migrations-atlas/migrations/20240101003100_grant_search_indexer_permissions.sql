-- Migration: grant search indexer permissions
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000031_grant_search_indexer_permissions.up.sql

-- Grant search indexer user access to all tables it needs for search functionality

-- Articles table access (for reading articles)
GRANT SELECT ON articles TO search_indexer_user;

-- Feed_tags table access (for reading tag information)
GRANT SELECT ON feed_tags TO search_indexer_user;

-- Article_tags table access (for reading article-tag relationships)
GRANT SELECT ON article_tags TO search_indexer_user;

-- Grant connect and usage permissions
GRANT CONNECT ON DATABASE alt TO search_indexer_user;
GRANT USAGE ON SCHEMA public TO search_indexer_user;
