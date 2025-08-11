-- Migration: add articles created id desc index
-- Created: 2025-08-12 00:19:20
-- Atlas Version: v0.35
-- Source: 000019_add_articles_created_id_desc_index.up.sql

CREATE INDEX idx_articles_created_id_desc
    ON articles (created_at DESC, id DESC);
