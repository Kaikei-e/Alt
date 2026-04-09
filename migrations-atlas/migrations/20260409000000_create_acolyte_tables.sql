-- Acolyte: Versioned report generation tables
-- Design: mutable current state + immutable version snapshots + explicit change tracking
-- See: refine.md for design rationale

-- reports: mutable current state (minimal fields)
CREATE TABLE reports (
    report_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    report_type TEXT NOT NULL DEFAULT 'weekly_briefing',
    current_version INT NOT NULL DEFAULT 0,
    latest_successful_run_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- report_versions: immutable snapshots (one row per version bump)
CREATE TABLE report_versions (
    report_id UUID NOT NULL REFERENCES reports(report_id),
    version_no INT NOT NULL,
    change_seq BIGSERIAL,
    change_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    prompt_template_version TEXT,
    scope_snapshot JSONB,
    outline_snapshot JSONB,
    summary_snapshot TEXT,
    PRIMARY KEY (report_id, version_no)
);
CREATE INDEX idx_report_versions_change_seq ON report_versions(change_seq);

-- report_change_items: field-level change tracking per version
CREATE TABLE report_change_items (
    report_id UUID NOT NULL,
    version_no INT NOT NULL,
    field_name TEXT NOT NULL,
    change_kind TEXT NOT NULL CHECK (change_kind IN ('added', 'updated', 'removed', 'regenerated')),
    old_fingerprint TEXT,
    new_fingerprint TEXT,
    PRIMARY KEY (report_id, version_no, field_name),
    FOREIGN KEY (report_id, version_no) REFERENCES report_versions(report_id, version_no)
);

-- report_sections: mutable section state
CREATE TABLE report_sections (
    report_id UUID NOT NULL REFERENCES reports(report_id),
    section_key TEXT NOT NULL,
    current_version INT NOT NULL DEFAULT 0,
    display_order INT NOT NULL DEFAULT 0,
    PRIMARY KEY (report_id, section_key)
);

-- report_section_versions: immutable section content snapshots
CREATE TABLE report_section_versions (
    report_id UUID NOT NULL,
    section_key TEXT NOT NULL,
    version_no INT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    citations_jsonb JSONB DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (report_id, section_key, version_no),
    FOREIGN KEY (report_id, section_key) REFERENCES report_sections(report_id, section_key)
);

-- report_runs: execution records (one per generation attempt)
CREATE TABLE report_runs (
    run_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    report_id UUID NOT NULL REFERENCES reports(report_id),
    target_version_no INT NOT NULL,
    run_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (run_status IN ('pending', 'running', 'succeeded', 'failed', 'cancelled')),
    planner_model TEXT,
    writer_model TEXT,
    critic_model TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    failure_code TEXT,
    failure_message TEXT
);
CREATE INDEX idx_report_runs_report_id ON report_runs(report_id);
CREATE INDEX idx_report_runs_active ON report_runs(run_status) WHERE run_status IN ('pending', 'running');

-- report_jobs: job queue with row-level locking (SELECT ... FOR UPDATE SKIP LOCKED)
CREATE TABLE report_jobs (
    job_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES report_runs(run_id),
    job_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (job_status IN ('pending', 'claimed', 'running', 'succeeded', 'failed')),
    attempt_no INT NOT NULL DEFAULT 0,
    claimed_by TEXT,
    claimed_at TIMESTAMPTZ,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_report_jobs_claimable ON report_jobs(available_at) WHERE job_status = 'pending';
