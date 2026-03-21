-- Eval framework tables

CREATE TABLE IF NOT EXISTS eval_exercises (
    id TEXT PRIMARY KEY,
    suite TEXT NOT NULL,
    language TEXT NOT NULL,
    slug TEXT NOT NULL,
    instructions TEXT NOT NULL,
    stub TEXT,
    test_file TEXT NOT NULL,
    test_command TEXT NOT NULL,
    docker_image TEXT NOT NULL,
    imported_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_eval_exercises_suite ON eval_exercises(suite);
CREATE INDEX IF NOT EXISTS idx_eval_exercises_language ON eval_exercises(language);
CREATE INDEX IF NOT EXISTS idx_eval_exercises_suite_language ON eval_exercises(suite, language);

CREATE TABLE IF NOT EXISTS eval_runs (
    id TEXT PRIMARY KEY,
    config TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at DATETIME NOT NULL,
    completed_at DATETIME,
    summary TEXT
);

CREATE INDEX IF NOT EXISTS idx_eval_runs_status ON eval_runs(status);
CREATE INDEX IF NOT EXISTS idx_eval_runs_started_at ON eval_runs(started_at DESC);

CREATE TABLE IF NOT EXISTS eval_results (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    exercise_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    model TEXT,
    pass_1 INTEGER DEFAULT 0,
    pass_2 INTEGER DEFAULT 0,
    generated_code TEXT,
    test_output TEXT,
    error_feedback TEXT,
    generated_code_2 TEXT,
    test_output_2 TEXT,
    latency_ms INTEGER,
    latency_ms_2 INTEGER,
    prompt_tokens INTEGER DEFAULT 0,
    completion_tokens INTEGER DEFAULT 0,
    total_tokens INTEGER DEFAULT 0,
    fallback_used INTEGER DEFAULT 0,
    fallback_chain TEXT,
    docker_exit_code INTEGER,
    error TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_eval_results_run_id ON eval_results(run_id);
CREATE INDEX IF NOT EXISTS idx_eval_results_exercise_id ON eval_results(exercise_id);
CREATE INDEX IF NOT EXISTS idx_eval_results_provider ON eval_results(provider);
