-- Usage tracking table
CREATE TABLE IF NOT EXISTS usage (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider TEXT NOT NULL,
    tokens INTEGER NOT NULL,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    request_id TEXT,
    model TEXT
);

CREATE INDEX IF NOT EXISTS idx_usage_provider_timestamp ON usage(provider, timestamp);
CREATE INDEX IF NOT EXISTS idx_usage_timestamp ON usage(timestamp);

-- Provider quotas configuration
CREATE TABLE IF NOT EXISTS provider_quotas (
    provider TEXT PRIMARY KEY,
    daily_limit INTEGER NOT NULL,
    monthly_limit INTEGER,
    reset_time DATETIME NOT NULL,
    tier TEXT,
    enabled INTEGER DEFAULT 1
);

-- Daily usage aggregates (for fast lookups)
CREATE TABLE IF NOT EXISTS daily_usage (
    provider TEXT NOT NULL,
    date DATE NOT NULL,
    total_tokens INTEGER DEFAULT 0,
    request_count INTEGER DEFAULT 0,
    PRIMARY KEY (provider, date)
);

CREATE INDEX IF NOT EXISTS idx_daily_usage_date ON daily_usage(date);

-- Vector memory for context storage (no compression)
CREATE TABLE IF NOT EXISTS memory (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    content TEXT NOT NULL,
    embedding BLOB,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    session_id TEXT,
    role TEXT,
    metadata TEXT
);

CREATE INDEX IF NOT EXISTS idx_memory_timestamp ON memory(timestamp);
CREATE INDEX IF NOT EXISTS idx_memory_session ON memory(session_id);

-- Request audit trail
CREATE TABLE IF NOT EXISTS request_audit (
    request_id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    selected_provider TEXT,
    final_provider TEXT,
    final_model TEXT,
    memory_query TEXT,
    memory_candidate_count INTEGER DEFAULT 0,
    success INTEGER NOT NULL,
    error_message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_request_audit_session_created
    ON request_audit(session_id, created_at DESC);

CREATE TABLE IF NOT EXISTS provider_attempt_audit (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    attempt_index INTEGER NOT NULL,
    success INTEGER NOT NULL,
    error_message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_provider_attempt_request
    ON provider_attempt_audit(request_id, attempt_index);

-- Orchestration persistent state
CREATE TABLE IF NOT EXISTS orchestration_tasks (
    id TEXT PRIMARY KEY,
    parent_task_id TEXT,
    goal TEXT NOT NULL,
    session_id TEXT NOT NULL,
    status TEXT NOT NULL,
    assigned_to TEXT,
    iteration INTEGER NOT NULL DEFAULT 1,
    feedback TEXT,
    created_at DATETIME NOT NULL,
    started_at DATETIME,
    completed_at DATETIME,
    error TEXT,
    final_output TEXT
);

CREATE TABLE IF NOT EXISTS orchestration_steps (
    task_id TEXT NOT NULL,
    step_id TEXT NOT NULL,
    role TEXT NOT NULL,
    prompt TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at DATETIME,
    completed_at DATETIME,
    output TEXT,
    error TEXT
);

CREATE INDEX IF NOT EXISTS idx_orchestration_steps_task ON orchestration_steps(task_id);

CREATE TABLE IF NOT EXISTS orchestration_agents (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    capabilities TEXT,
    status TEXT NOT NULL,
    swarm_id TEXT,
    created_at DATETIME NOT NULL,
    last_seen_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS orchestration_swarms (
    id TEXT PRIMARY KEY,
    objective TEXT NOT NULL,
    topology TEXT NOT NULL,
    strategy TEXT NOT NULL,
    status TEXT NOT NULL,
    max_agents INTEGER NOT NULL,
    agent_ids TEXT,
    task_ids TEXT,
    created_at DATETIME NOT NULL,
    started_at DATETIME,
    finished_at DATETIME
);

-- Runtime settings for compatibility layers
CREATE TABLE IF NOT EXISTS runtime_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Circuit breaker state
CREATE TABLE IF NOT EXISTS circuit_breaker_state (
    provider TEXT PRIMARY KEY,
    state TEXT NOT NULL, -- 'closed', 'open', 'half_open'
    failure_count INTEGER DEFAULT 0,
    last_failure_time DATETIME,
    open_until DATETIME
);

-- Insert default provider quotas (update with your actual limits)
INSERT OR IGNORE INTO provider_quotas (provider, daily_limit, monthly_limit, reset_time, tier) VALUES
    ('claude-code', 500000, 15000000, datetime('now', '+1 day'), 'pro'),
    ('codex', 300000, 9000000, datetime('now', '+1 day'), 'pro'),
    ('gemini', 500000, 15000000, datetime('now', '+1 day'), 'pro'),
    ('qwen', 500000, 15000000, datetime('now', '+1 day'), 'pro'),
    ('ollama-cloud', 1000000, 30000000, datetime('now', '+1 day'), 'pro'),
    ('nanogpt', 2000000, 60000000, datetime('now', '+1 month'), 'subscription');

-- Initialize circuit breakers (all closed)
INSERT OR IGNORE INTO circuit_breaker_state (provider, state, failure_count) VALUES
    ('claude-code', 'closed', 0),
    ('codex', 'closed', 0),
    ('gemini', 'closed', 0),
    ('qwen', 'closed', 0),
    ('ollama-cloud', 'closed', 0),
    ('nanogpt', 'closed', 0);
