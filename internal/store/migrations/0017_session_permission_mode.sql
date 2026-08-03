-- The session's permission mode (default/acceptEdits/plan/auto/bypassPermissions),
-- surfaced from the hook payload and the transcript; empty when unknown (#304).
ALTER TABLE sessions ADD COLUMN permission_mode TEXT NOT NULL DEFAULT '';
