-- Add tool_call_log column to agent_sessions for durable execution checkpointing.
-- Stores JSON array of completed tool call IDs so resumed sessions skip completed work.
ALTER TABLE agent_sessions ADD COLUMN tool_call_log TEXT DEFAULT '[]';

-- Add background_tasks table for --background flag support.
CREATE TABLE IF NOT EXISTS background_tasks (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'pending',
    worktree_path TEXT,
    message TEXT,
    pr_url TEXT,
    error TEXT,
    start_time DATETIME DEFAULT CURRENT_TIMESTAMP,
    end_time DATETIME
);
