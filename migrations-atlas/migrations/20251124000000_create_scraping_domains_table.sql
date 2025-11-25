-- Migration: Create scraping_domains table
-- Created: 2025-11-24
-- Description: Creates the scraping_domains table for domain-level scraping policy and robots.txt cache management

CREATE TABLE IF NOT EXISTS scraping_domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain TEXT NOT NULL,
    scheme TEXT NOT NULL DEFAULT 'https',
    allow_fetch_body BOOLEAN NOT NULL DEFAULT true,
    allow_ml_training BOOLEAN NOT NULL DEFAULT true,
    allow_cache_days INTEGER NOT NULL DEFAULT 7,
    force_respect_robots BOOLEAN NOT NULL DEFAULT true,
    robots_txt_url TEXT,
    robots_txt_content TEXT,
    robots_txt_fetched_at TIMESTAMPTZ,
    robots_txt_last_status INTEGER,
    robots_crawl_delay_sec INTEGER,
    robots_disallow_paths JSONB DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT scraping_domains_domain_key UNIQUE (domain)
);

COMMENT ON TABLE scraping_domains IS 'Domain-level scraping policy and robots.txt cache';
COMMENT ON COLUMN scraping_domains.domain IS 'Domain name (e.g., example.com)';
COMMENT ON COLUMN scraping_domains.scheme IS 'Protocol scheme (http or https)';
COMMENT ON COLUMN scraping_domains.allow_fetch_body IS 'Whether to allow fetching article bodies';
COMMENT ON COLUMN scraping_domains.allow_ml_training IS 'Whether to allow using content for ML training/summarization';
COMMENT ON COLUMN scraping_domains.allow_cache_days IS 'Days to keep article body in cache';
COMMENT ON COLUMN scraping_domains.force_respect_robots IS 'Whether to strictly respect robots.txt';

