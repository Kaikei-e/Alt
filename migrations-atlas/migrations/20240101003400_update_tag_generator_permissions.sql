-- Migration: update tag generator permissions
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000034_update_tag_generator_permissions.up.sql

-- Reset tag_generator privileges and grant only the required access

-- Remove any existing privileges on the database to start from a clean state
REVOKE ALL ON DATABASE alt FROM tag_generator;

-- Basic connectivity and schema usage privileges
GRANT CONNECT ON DATABASE alt TO tag_generator;
GRANT USAGE ON SCHEMA public TO tag_generator;

-- Table-level privileges
GRANT SELECT ON articles TO tag_generator;
GRANT SELECT, INSERT ON tags TO tag_generator;
GRANT SELECT, INSERT ON article_tags TO tag_generator;
