-- Align recap_preprocess_metrics schema with Recap Worker DAO expectations.
BEGIN;

-- Preserve the legacy table for data migration.
ALTER TABLE recap_preprocess_metrics
    RENAME TO recap_preprocess_metrics_legacy;

CREATE TABLE recap_preprocess_metrics (
    job_id UUID PRIMARY KEY REFERENCES recap_jobs(job_id) ON DELETE CASCADE,
    total_articles_fetched INTEGER NOT NULL,
    articles_processed INTEGER NOT NULL,
    articles_dropped_empty INTEGER NOT NULL,
    articles_html_cleaned INTEGER NOT NULL,
    total_characters BIGINT NOT NULL,
    avg_chars_per_article DOUBLE PRECISION,
    languages_detected JSONB NOT NULL DEFAULT '{}'::jsonb
);

WITH raw AS (
    SELECT
        job_id,
        metric,
        value,
        NULLIF(value ->> 'count', '') AS count_text,
        NULLIF(value ->> 'value', '') AS value_text,
        NULLIF(trim(both '"' FROM value::text), '') AS bare_text
    FROM recap_preprocess_metrics_legacy
),
numeric_values AS (
    SELECT
        job_id,
        metric,
        value,
        CASE
            WHEN count_text ~ E'^-?[0-9]+(\\.[0-9]+)?$' THEN count_text::numeric
            WHEN value_text ~ E'^-?[0-9]+(\\.[0-9]+)?$' THEN value_text::numeric
            WHEN bare_text ~ E'^-?[0-9]+(\\.[0-9]+)?$' THEN bare_text::numeric
            ELSE NULL
        END AS numeric_value
    FROM raw
),
aggregated AS (
    SELECT
        job_id,
        COALESCE(
            MAX(numeric_value) FILTER (WHERE metric IN ('total_articles_fetched', 'total_articles')),
            0
        )::INTEGER AS total_articles_fetched,
        COALESCE(
            MAX(numeric_value) FILTER (WHERE metric IN ('articles_processed', 'processed_count')),
            0
        )::INTEGER AS articles_processed,
        COALESCE(
            MAX(numeric_value) FILTER (WHERE metric IN ('articles_dropped_empty', 'articles_dropped', 'dropped_count')),
            0
        )::INTEGER AS articles_dropped_empty,
        COALESCE(
            MAX(numeric_value) FILTER (WHERE metric IN ('articles_html_cleaned', 'html_cleaned_count')),
            0
        )::INTEGER AS articles_html_cleaned,
        COALESCE(
            MAX(numeric_value) FILTER (WHERE metric IN ('total_characters', 'character_count')),
            0
        )::BIGINT AS total_characters,
        (MAX(numeric_value) FILTER (WHERE metric IN ('avg_chars_per_article', 'avg_chars')))::DOUBLE PRECISION
            AS avg_chars_per_article,
        COALESCE(
            (array_agg(value ORDER BY metric) FILTER (WHERE metric IN ('languages_detected', 'language_counts')))[1],
            '{}'::jsonb
        ) AS languages_detected
    FROM numeric_values
    GROUP BY job_id
)
INSERT INTO recap_preprocess_metrics (
    job_id,
    total_articles_fetched,
    articles_processed,
    articles_dropped_empty,
    articles_html_cleaned,
    total_characters,
    avg_chars_per_article,
    languages_detected
)
SELECT
    job_id,
    total_articles_fetched,
    articles_processed,
    articles_dropped_empty,
    articles_html_cleaned,
    total_characters,
    avg_chars_per_article,
    languages_detected
FROM aggregated;

DROP TABLE recap_preprocess_metrics_legacy;

COMMIT;

