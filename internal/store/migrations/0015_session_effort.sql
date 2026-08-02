-- The reasoning effort of the session's last assistant turn (low/medium/high/
-- xhigh/max), surfaced from the transcript; empty when unknown (#286).
ALTER TABLE sessions ADD COLUMN effort TEXT NOT NULL DEFAULT '';
