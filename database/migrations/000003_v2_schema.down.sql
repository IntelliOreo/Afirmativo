-- Reverse migration 000003: drop new tables, remove new columns, restore old columns.

DROP TABLE IF EXISTS answers;
DROP TABLE IF EXISTS question_areas;

DROP INDEX IF EXISTS idx_sessions_status;

ALTER TABLE sessions DROP COLUMN IF EXISTS interview_budget_seconds;
ALTER TABLE sessions DROP COLUMN IF EXISTS interview_lapsed_seconds;
ALTER TABLE sessions DROP COLUMN IF EXISTS interview_lapsed_updated_at;
ALTER TABLE sessions DROP COLUMN IF EXISTS interview_started_at;
ALTER TABLE sessions DROP COLUMN IF EXISTS current_interview_started_at;
ALTER TABLE sessions DROP COLUMN IF EXISTS last_api_call_at;
ALTER TABLE sessions DROP COLUMN IF EXISTS conversation_history;

ALTER TABLE sessions ADD COLUMN timer_seconds INT NOT NULL DEFAULT 3600;
ALTER TABLE sessions ADD COLUMN started_at TIMESTAMPTZ;
