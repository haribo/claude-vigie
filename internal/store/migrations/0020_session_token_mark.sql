-- Per-session high-water mark of output tokens already counted into stats_daily.
--
-- It lives here rather than on the sessions row because a session row is pruned
-- by retention while its transcript survives: resuming such a session made the
-- rollup re-add the whole lifetime total to that day (#432). This table is never
-- pruned, for the same reason stats_daily is not — it is what keeps that history
-- correct. Cost is one session id and one integer per session ever seen.
-- See docs/design/token-rollup.md.
CREATE TABLE session_token_mark (
    session_id TEXT PRIMARY KEY,
    counted    INTEGER NOT NULL DEFAULT 0
);
