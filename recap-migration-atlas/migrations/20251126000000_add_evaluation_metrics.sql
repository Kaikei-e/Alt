-- Add extended evaluation metrics to recap_genre_evaluation_runs table
-- These metrics provide additional statistical insights for evaluation validity

ALTER TABLE recap_genre_evaluation_runs
ADD COLUMN IF NOT EXISTS micro_precision DOUBLE PRECISION,
ADD COLUMN IF NOT EXISTS micro_recall DOUBLE PRECISION,
ADD COLUMN IF NOT EXISTS micro_f1 DOUBLE PRECISION,
ADD COLUMN IF NOT EXISTS weighted_f1 DOUBLE PRECISION,
ADD COLUMN IF NOT EXISTS macro_f1_valid DOUBLE PRECISION,
ADD COLUMN IF NOT EXISTS valid_genre_count INTEGER,
ADD COLUMN IF NOT EXISTS undefined_genre_count INTEGER;

-- Add comments for documentation
COMMENT ON COLUMN recap_genre_evaluation_runs.micro_precision IS 'Micro-averaged precision: total TP / (total TP + total FP)';
COMMENT ON COLUMN recap_genre_evaluation_runs.micro_recall IS 'Micro-averaged recall: total TP / (total TP + total FN)';
COMMENT ON COLUMN recap_genre_evaluation_runs.micro_f1 IS 'Micro-averaged F1: harmonic mean of micro precision and micro recall';
COMMENT ON COLUMN recap_genre_evaluation_runs.weighted_f1 IS 'Weighted F1: F1 scores weighted by support (TP + FN) for each genre';
COMMENT ON COLUMN recap_genre_evaluation_runs.macro_f1_valid IS 'Macro F1 excluding genres with no support (TP + FN = 0)';
COMMENT ON COLUMN recap_genre_evaluation_runs.valid_genre_count IS 'Number of genres with support > 0 used in macro_f1_valid calculation';
COMMENT ON COLUMN recap_genre_evaluation_runs.undefined_genre_count IS 'Number of genres with no support (TP + FN = 0) excluded from macro_f1_valid';

