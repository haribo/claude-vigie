CREATE TABLE token_samples (
    session_id    TEXT NOT NULL,
    at            TEXT NOT NULL,
    output_tokens INTEGER NOT NULL,
    PRIMARY KEY (session_id, at)
);
