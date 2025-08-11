-- Migration: grant preprocessor delete article summaries
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000024_grant_preprocessor_delete_article_summaries.up.sql

-- Grant DELETE permission on article_summaries table to the preprocessor user
GRANT DELETE ON TABLE article_summaries TO pre_processor_user;
