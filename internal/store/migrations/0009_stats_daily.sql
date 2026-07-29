-- Daily activity rollups, aggregated incrementally as reports arrive. This
-- table is never pruned, so it retains history beyond the session-retention
-- window (see docs: analytics approach b).
CREATE TABLE stats_daily (
    day             TEXT NOT NULL,           -- UTC calendar day, YYYY-MM-DD
    model           TEXT NOT NULL DEFAULT '',
    output_tokens   INTEGER NOT NULL DEFAULT 0,
    working_seconds INTEGER NOT NULL DEFAULT 0,
    waiting_seconds INTEGER NOT NULL DEFAULT 0,
    idle_seconds    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (day, model)
);
