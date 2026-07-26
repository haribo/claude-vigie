CREATE TABLE sessions (
    id                    TEXT PRIMARY KEY,
    machine               TEXT NOT NULL,
    project_dir           TEXT NOT NULL,
    git_branch            TEXT NOT NULL DEFAULT '',
    model                 TEXT NOT NULL DEFAULT '',
    status                TEXT NOT NULL,
    last_tool             TEXT NOT NULL DEFAULT '',
    input_tokens          INTEGER NOT NULL DEFAULT 0,
    output_tokens         INTEGER NOT NULL DEFAULT 0,
    cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens     INTEGER NOT NULL DEFAULT 0,
    started_at            TEXT NOT NULL,
    last_seen_at          TEXT NOT NULL,
    ended_at              TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_sessions_status ON sessions (status);
CREATE INDEX idx_sessions_machine ON sessions (machine);

CREATE TABLE events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL REFERENCES sessions (id),
    event      TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX idx_events_session_id ON events (session_id);
