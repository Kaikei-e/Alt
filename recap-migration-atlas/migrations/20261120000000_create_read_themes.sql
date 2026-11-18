CREATE TABLE read_theme_runs (
    run_id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    kicked_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NULL,
    status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'failed')),
    source_count INT NOT NULL DEFAULT 0,
    versions_created INT NOT NULL DEFAULT 0,
    versions_deferred INT NOT NULL DEFAULT 0,
    params_version TEXT NOT NULL,
    error_message TEXT NULL
);

CREATE INDEX idx_read_theme_runs_user_kicked
    ON read_theme_runs (user_id, kicked_at DESC);

CREATE TABLE read_themes (
    user_id UUID NOT NULL,
    theme_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, theme_id)
);

CREATE TABLE read_theme_versions (
    version_id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    theme_id UUID NOT NULL,
    version INT NOT NULL CHECK (version >= 1),
    run_id UUID NOT NULL REFERENCES read_theme_runs (run_id),
    title TEXT NOT NULL,
    member_article_ids UUID[] NOT NULL,
    membership_hash TEXT NOT NULL,
    centroid REAL[] NOT NULL,
    body_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (user_id, theme_id, version),
    FOREIGN KEY (user_id, theme_id) REFERENCES read_themes (user_id, theme_id)
);

CREATE INDEX idx_read_theme_versions_run_id
    ON read_theme_versions (run_id);

CREATE INDEX idx_read_theme_versions_user_theme_version_desc
    ON read_theme_versions (user_id, theme_id, version DESC);

CREATE OR REPLACE FUNCTION prevent_read_theme_version_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'read_theme_versions is append-only: % is forbidden', TG_OP;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_read_theme_versions_immutable
    BEFORE UPDATE OR DELETE ON read_theme_versions
    FOR EACH ROW
    EXECUTE FUNCTION prevent_read_theme_version_mutation();

CREATE TABLE read_theme_version_publications (
    version_id UUID PRIMARY KEY REFERENCES read_theme_versions (version_id),
    published_at TIMESTAMPTZ NOT NULL
);
