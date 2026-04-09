-- report_briefs: typed input specification for report generation
-- Separate from reports table to follow JSONB-for-auxiliary-only rule.
-- topic/report_type are queryable TEXT fields, not JSONB.
CREATE TABLE report_briefs (
    report_id UUID PRIMARY KEY REFERENCES reports(report_id),
    topic TEXT NOT NULL,
    report_type TEXT NOT NULL,
    time_range TEXT,
    entities TEXT[] NOT NULL DEFAULT '{}',
    exclude_topics TEXT[] NOT NULL DEFAULT '{}',
    constraints_jsonb JSONB NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_report_briefs_topic ON report_briefs(topic);
