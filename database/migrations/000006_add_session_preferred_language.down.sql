ALTER TABLE sessions
DROP CONSTRAINT IF EXISTS chk_sessions_preferred_language;

ALTER TABLE sessions
DROP COLUMN IF EXISTS preferred_language;
