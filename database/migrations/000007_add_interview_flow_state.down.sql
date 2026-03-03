DROP TABLE IF EXISTS interview_events;

DROP INDEX IF EXISTS idx_sessions_flow_step;

ALTER TABLE sessions DROP CONSTRAINT IF EXISTS chk_sessions_display_question_number;
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS chk_sessions_flow_step;

ALTER TABLE sessions DROP COLUMN IF EXISTS display_question_number;
ALTER TABLE sessions DROP COLUMN IF EXISTS expected_turn_id;
ALTER TABLE sessions DROP COLUMN IF EXISTS flow_version;
ALTER TABLE sessions DROP COLUMN IF EXISTS flow_step;
