-- Tag label graph table for Recap refinement
CREATE TABLE IF NOT EXISTS tag_label_graph (
    window_label TEXT NOT NULL,
    genre TEXT NOT NULL,
    tag TEXT NOT NULL,
    weight REAL NOT NULL CHECK (weight >= 0 AND weight <= 1),
    sample_size INTEGER NOT NULL DEFAULT 0 CHECK (sample_size >= 0),
    last_observed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (window_label, genre, tag)
);

CREATE INDEX IF NOT EXISTS idx_tag_label_graph_genre
    ON tag_label_graph (genre, tag);
