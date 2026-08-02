-- The /rc resume URL (https://claude.ai/code/session_…) surfaced from the
-- session registry's bridgeSessionId; empty when remote control is off (#253).
ALTER TABLE sessions ADD COLUMN remote_url TEXT NOT NULL DEFAULT '';
