-- Migration: grant preprocessor article summaries permissions
-- Created: 2025-08-12 00:19:20
-- Atlas Version: v0.35
-- Source: 000011_grant_preprocessor_article_summaries_permissions.up.sql

-- Grant permissions on article_summaries table to the preprocessor user
GRANT SELECT, INSERT, UPDATE ON TABLE article_summaries TO pre_processor_user;
