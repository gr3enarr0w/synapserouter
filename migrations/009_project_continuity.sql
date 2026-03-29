-- Project continuity: cross-session state for synroute code sessions
CREATE TABLE IF NOT EXISTS project_continuity (
    project_dir   TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL,
    phase         TEXT DEFAULT '',
    build_status  TEXT DEFAULT '',
    test_status   TEXT DEFAULT '',
    language      TEXT DEFAULT '',
    model         TEXT DEFAULT '',
    file_manifest TEXT DEFAULT '[]',
    context_summary TEXT DEFAULT '',
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
