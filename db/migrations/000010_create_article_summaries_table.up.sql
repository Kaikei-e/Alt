CREATE TABLE IF NOT EXISTS article_summaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    article_id UUID NOT NULL,
    article_title TEXT NOT NULL,
    summary_japanese TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_article_summaries_article_id
        FOREIGN KEY (article_id)
        REFERENCES articles(id)
        ON DELETE CASCADE
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_article_summaries_article_id ON article_summaries (article_id);
CREATE INDEX IF NOT EXISTS idx_article_summaries_created_at ON article_summaries (created_at);

-- Ensure only one summary per article
CREATE UNIQUE INDEX IF NOT EXISTS idx_article_summaries_unique_article ON article_summaries (article_id);