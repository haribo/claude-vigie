CREATE TABLE usage_lease (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    holder     TEXT NOT NULL,
    expires_at TEXT NOT NULL
);
