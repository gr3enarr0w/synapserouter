-- Add tool_call_id and source tracking to memory table
-- Note: ALTER TABLE ADD COLUMN will error on re-run (duplicate column).
-- The migration runner handles this gracefully by checking for "duplicate column" errors.
ALTER TABLE memory ADD COLUMN tool_call_id TEXT;
ALTER TABLE memory ADD COLUMN source TEXT DEFAULT 'manual';
CREATE INDEX IF NOT EXISTS idx_memory_session_role ON memory(session_id, role);
CREATE INDEX IF NOT EXISTS idx_memory_source ON memory(session_id, source);
