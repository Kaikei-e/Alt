-- Migration: grant tag generator permissions
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000028_grant_tag_generator_permissions.up.sql

GRANT SELECT, INSERT, DELETE
    ON feed_tags, article_tags
    TO tag_generator;
