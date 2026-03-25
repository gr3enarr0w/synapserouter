-- Add tool_call_id and source tracking to memory table
-- Enables preserving tool call structure during compaction
-- and tracking where stored messages came from (compaction, auto_trim, overflow)
ALTER TABLE memory ADD COLUMN tool_call_id TEXT;
ALTER TABLE memory ADD COLUMN source TEXT DEFAULT 'manual';
CREATE INDEX IF NOT EXISTS idx_memory_session_role ON memory(session_id, role);
CREATE INDEX IF NOT EXISTS idx_memory_source ON memory(session_id, source);
