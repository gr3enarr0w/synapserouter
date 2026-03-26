-- Tool output storage: full output preserved in DB while conversation gets summaries.
-- Enables the recall tool to retrieve past tool results by query or tool name.
CREATE TABLE IF NOT EXISTS tool_outputs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    args_summary TEXT,
    summary TEXT NOT NULL,
    full_output TEXT,
    exit_code INTEGER DEFAULT 0,
    output_size INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_tool_outputs_session ON tool_outputs(session_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tool_outputs_tool ON tool_outputs(session_id, tool_name);
