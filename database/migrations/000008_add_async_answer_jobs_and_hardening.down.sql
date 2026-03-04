-- Migration 000008 down: remove async jobs and revert hardening changes.

-- 1) Drop async jobs table and indexes.
DROP INDEX IF EXISTS idx_interview_answer_jobs_updated_at;
DROP INDEX IF EXISTS idx_interview_answer_jobs_session_created_at;
DROP INDEX IF EXISTS idx_interview_answer_jobs_status_created_at;
DROP INDEX IF EXISTS uq_interview_answer_jobs_session_request;
DROP TABLE IF EXISTS interview_answer_jobs;

-- 2) Restore dropped indexes.
CREATE INDEX idx_reports_session_code ON reports(session_code);
CREATE INDEX idx_interview_events_session_code ON interview_events(session_code);

-- 3) Revert FK delete behavior to non-cascading references.
ALTER TABLE reports DROP CONSTRAINT IF EXISTS reports_session_code_fkey;
ALTER TABLE reports
    ADD CONSTRAINT reports_session_code_fkey
    FOREIGN KEY (session_code) REFERENCES sessions(session_code);

ALTER TABLE answers DROP CONSTRAINT IF EXISTS answers_session_code_fkey;
ALTER TABLE answers
    ADD CONSTRAINT answers_session_code_fkey
    FOREIGN KEY (session_code) REFERENCES sessions(session_code);

ALTER TABLE question_areas DROP CONSTRAINT IF EXISTS question_areas_session_code_fkey;
ALTER TABLE question_areas
    ADD CONSTRAINT question_areas_session_code_fkey
    FOREIGN KEY (session_code) REFERENCES sessions(session_code);

-- 4) Remove added status constraints.
ALTER TABLE question_areas DROP CONSTRAINT IF EXISTS chk_question_areas_status;
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS chk_sessions_status;

-- 5) Restore flow version marker.
ALTER TABLE sessions ADD COLUMN flow_version INT NOT NULL DEFAULT 2;
