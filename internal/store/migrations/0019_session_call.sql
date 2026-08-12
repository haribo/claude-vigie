-- A call the session raised for the operator (#388, ADR-0010): the message and
-- when it was raised. Orthogonal to status — a calling session keeps whatever
-- status it has, and the registry status enum stays closed. A call is present
-- iff call_at is non-empty. It is set and cleared by the session itself
-- (UserPromptSubmit, SessionEnd), never by an action on vigie (ADR-0007).
ALTER TABLE sessions ADD COLUMN call_message TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN call_at TEXT NOT NULL DEFAULT '';
