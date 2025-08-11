-- Migration: create feed links
-- Created: 2025-08-12 00:19:20
-- Atlas Version: v0.35
-- Source: 000002_create_feed_links.up.sql

CREATE TABLE IF NOT EXISTS feed_links (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    url TEXT NOT NULL UNIQUE,
    CONSTRAINT idx_feed_links_id_url UNIQUE (id, url)
);
