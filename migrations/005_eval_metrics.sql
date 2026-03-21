-- Eval framework extensions for metric-based scoring and LLM judge modes
-- NOTE: This migration is now handled programmatically in ensureEvalMetricColumns()
-- to work around SQLite's lack of ADD COLUMN IF NOT EXISTS.
-- Keeping this file as a no-op so the migration runner doesn't break on missing files.
SELECT 1;
