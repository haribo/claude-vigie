-- Whether context_tokens is a *known* reading (1) or merely absent (0). Splits
-- known-empty (a just-cleared session vigie parsed and found at 0% — a truthful
-- reading) from unknown (no context reading at all, rendered "-"). Historically
-- 0 tokens meant "unknown"; existing rows keep that meaning — a positive count
-- was known, a 0 was not (#367).
ALTER TABLE sessions ADD COLUMN context_known INTEGER NOT NULL DEFAULT 0;
UPDATE sessions SET context_known = 1 WHERE context_tokens > 0;
