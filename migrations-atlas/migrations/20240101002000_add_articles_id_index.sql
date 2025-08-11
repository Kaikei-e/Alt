-- Migration: add articles id index
-- Created: 2025-08-12 00:19:20
-- Atlas Version: v0.35
-- Source: 000020_add_articles_id_index.up.sql

CREATE INDEX IF NOT EXISTS idx_articles_id_only ON articles (id);
