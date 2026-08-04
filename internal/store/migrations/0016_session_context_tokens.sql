-- The real prompt size of the session's latest request (input + cache-read +
-- cache-creation tokens), compared against the model's context window to show
-- how full the context is (#279). 0 when unknown.
ALTER TABLE sessions ADD COLUMN context_tokens INTEGER NOT NULL DEFAULT 0;
