-- Create genre evaluation run metadata table
CREATE TABLE recap_genre_evaluation_runs (
    run_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dataset_path TEXT NOT NULL,
    total_items INTEGER NOT NULL,
    macro_precision DOUBLE PRECISION NOT NULL,
    macro_recall DOUBLE PRECISION NOT NULL,
    macro_f1 DOUBLE PRECISION NOT NULL,
    summary_tp INTEGER NOT NULL,
    summary_fp INTEGER NOT NULL,
    summary_fn INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create per-genre evaluation metrics table
CREATE TABLE recap_genre_evaluation_metrics (
    run_id UUID NOT NULL REFERENCES recap_genre_evaluation_runs(run_id) ON DELETE CASCADE,
    genre TEXT NOT NULL,
    tp INTEGER NOT NULL,
    fp INTEGER NOT NULL,
    fn_count INTEGER NOT NULL,
    precision DOUBLE PRECISION NOT NULL,
    recall DOUBLE PRECISION NOT NULL,
    f1_score DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (run_id, genre)
);

-- Indexes for efficient querying
CREATE INDEX idx_recap_genre_evaluation_runs_created_at
    ON recap_genre_evaluation_runs(created_at DESC);

CREATE INDEX idx_recap_genre_evaluation_metrics_run_id
    ON recap_genre_evaluation_metrics(run_id);

CREATE INDEX idx_recap_genre_evaluation_metrics_genre
    ON recap_genre_evaluation_metrics(genre);

-- Comments for documentation
COMMENT ON TABLE recap_genre_evaluation_runs IS 'Stores metadata for genre classification evaluation runs using golden datasets';
COMMENT ON TABLE recap_genre_evaluation_metrics IS 'Stores per-genre precision, recall, and F1 metrics for each evaluation run';
COMMENT ON COLUMN recap_genre_evaluation_runs.dataset_path IS 'Path to the golden dataset JSON file used for this evaluation';
COMMENT ON COLUMN recap_genre_evaluation_metrics.fn_count IS 'False negative count for this genre';

