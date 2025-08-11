-- Migration: optimize articles title indexes
-- Created: 2025-08-12 00:19:20
-- Atlas Version: v0.35
-- Source: 000022_optimize_articles_title_indexes.up.sql

DROP INDEX IF EXISTS idx_articles_title_created_at;
