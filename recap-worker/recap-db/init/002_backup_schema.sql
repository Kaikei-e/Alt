-- ジョブの起点（1レコード = 04:00実行1回）
CREATE TABLE IF NOT EXISTS recap_jobs (
    id         BIGSERIAL PRIMARY KEY,
    job_id     UUID NOT NULL UNIQUE,
    kicked_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    note       TEXT
);

-- 取得した生記事のスナップショット（HTML含む、再現性のため保存）
CREATE TABLE IF NOT EXISTS recap_job_articles (
    id            BIGSERIAL PRIMARY KEY,
    job_id        UUID NOT NULL REFERENCES recap_jobs(job_id) ON DELETE CASCADE,
    article_id    TEXT NOT NULL,
    title         TEXT,
    fulltext_html TEXT NOT NULL,
    published_at  TIMESTAMPTZ,
    source_url    TEXT,
    lang_hint     TEXT,
    normalized_hash TEXT NOT NULL,    -- 正規化テキストのハッシュ(重複検出)
    UNIQUE (job_id, article_id)
);
CREATE INDEX IF NOT EXISTS idx_recap_job_articles_job
    ON recap_job_articles (job_id);

-- 前処理の統計（CPUバウンド：重複率、文数、除外理由など）
CREATE TABLE IF NOT EXISTS recap_preprocess_metrics (
    job_id                  UUID PRIMARY KEY REFERENCES recap_jobs(job_id) ON DELETE CASCADE,
    total_articles_fetched  INTEGER NOT NULL,
    articles_processed      INTEGER NOT NULL,
    articles_dropped_empty  INTEGER NOT NULL,
    articles_html_cleaned   INTEGER NOT NULL,
    total_characters        BIGINT NOT NULL,
    avg_chars_per_article   DOUBLE PRECISION,
    languages_detected      JSONB NOT NULL DEFAULT '{}'::jsonb
);
