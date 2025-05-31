CREATE DATABASE IF NOT EXISTS alt_db;

USE alt_db;

CREATE TABLE feeds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    link TEXT NOT NULL,
    pub_date TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_feeds_created_at (created_at)
    INDEX idx_feeds_id_link (id, link)
);