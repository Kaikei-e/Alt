-- Migration: create articles table
-- Created: 2025-08-12 00:19:20
-- Atlas Version: v0.35
-- Source: 000007_create_articles_table.up.sql

CREATE TABLE IF NOT EXISTS articles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    url TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_articles_created_at ON articles (created_at);
CREATE INDEX IF NOT EXISTS idx_articles_title_created_at ON articles (title, created_at);
CREATE INDEX IF NOT EXISTS idx_articles_title ON articles (title);
