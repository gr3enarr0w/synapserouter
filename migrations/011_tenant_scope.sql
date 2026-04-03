-- Add user_id column for multi-tenant support (defaults to 'local' for single-user)
-- Allows future multi-user deployment without schema rewrite

ALTER TABLE agent_sessions ADD COLUMN user_id TEXT DEFAULT 'local';
ALTER TABLE background_tasks ADD COLUMN user_id TEXT DEFAULT 'local';
ALTER TABLE memory ADD COLUMN user_id TEXT DEFAULT 'local';

CREATE INDEX IF NOT EXISTS idx_agent_sessions_user ON agent_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_background_tasks_user ON background_tasks(user_id);
CREATE INDEX IF NOT EXISTS idx_memory_user ON memory(user_id);
