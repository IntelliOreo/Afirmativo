ALTER TABLE sessions
ADD COLUMN preferred_language TEXT;

ALTER TABLE sessions
ADD CONSTRAINT chk_sessions_preferred_language
CHECK (preferred_language IN ('es', 'en'));
