-- Create morning_daily_summaries table
CREATE TABLE morning_daily_summaries (
    id UUID PRIMARY KEY,
    target_date DATE NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_morning_daily_summaries_target_date ON morning_daily_summaries(target_date);

-- Create morning_daily_evidence table
CREATE TABLE morning_daily_evidence (
    summary_id UUID NOT NULL REFERENCES morning_daily_summaries(id) ON DELETE CASCADE,
    article_id UUID NOT NULL, -- References articles table in a separate DB/Schema conceptually, but here just UUID
    score FLOAT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (summary_id, article_id)
);

-- Create morning_article_groups table
CREATE TABLE morning_article_groups (
    group_id UUID NOT NULL,
    article_id UUID NOT NULL,
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_id, article_id)
);

CREATE INDEX idx_morning_article_groups_created_at ON morning_article_groups(created_at);
