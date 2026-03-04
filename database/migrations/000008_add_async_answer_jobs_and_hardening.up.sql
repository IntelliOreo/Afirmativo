-- Migration 000008: Add async interview answer jobs and integrity hardening.

-- 1) Remove flow version marker (no longer used by runtime logic).
ALTER TABLE sessions DROP COLUMN IF EXISTS flow_version;

-- 2) Add status guards to prevent invalid state values.
ALTER TABLE sessions
    ADD CONSTRAINT chk_sessions_status
    CHECK (status IN ('created', 'paying', 'active', 'interviewing', 'completed', 'expired'));

ALTER TABLE question_areas
    ADD CONSTRAINT chk_question_areas_status
    CHECK (status IN ('pending', 'pre_addressed', 'in_progress', 'complete', 'insufficient', 'not_assessed'));

-- 3) Tighten referential integrity cleanup semantics.
ALTER TABLE question_areas DROP CONSTRAINT IF EXISTS question_areas_session_code_fkey;
ALTER TABLE question_areas
    ADD CONSTRAINT question_areas_session_code_fkey
    FOREIGN KEY (session_code) REFERENCES sessions(session_code) ON DELETE CASCADE;

ALTER TABLE answers DROP CONSTRAINT IF EXISTS answers_session_code_fkey;
ALTER TABLE answers
    ADD CONSTRAINT answers_session_code_fkey
    FOREIGN KEY (session_code) REFERENCES sessions(session_code) ON DELETE CASCADE;

ALTER TABLE reports DROP CONSTRAINT IF EXISTS reports_session_code_fkey;
ALTER TABLE reports
    ADD CONSTRAINT reports_session_code_fkey
    FOREIGN KEY (session_code) REFERENCES sessions(session_code) ON DELETE CASCADE;

-- 4) Drop redundant indexes now covered by stricter keys.
DROP INDEX IF EXISTS idx_reports_session_code;
DROP INDEX IF EXISTS idx_interview_events_session_code;

-- 5) Async submit + poll durable job table.
CREATE TABLE interview_answer_jobs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_code      TEXT NOT NULL REFERENCES sessions(session_code) ON DELETE CASCADE,
    client_request_id TEXT NOT NULL,
    turn_id           TEXT NOT NULL,
    question_text     TEXT,
    answer_text       TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'queued',
    result_payload    JSONB,
    error_code        TEXT,
    error_message     TEXT,
    attempts          INT NOT NULL DEFAULT 0,
    started_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_interview_answer_jobs_status
        CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'conflict', 'canceled'))
);

CREATE UNIQUE INDEX uq_interview_answer_jobs_session_request
    ON interview_answer_jobs(session_code, client_request_id);

CREATE INDEX idx_interview_answer_jobs_status_created_at
    ON interview_answer_jobs(status, created_at);

CREATE INDEX idx_interview_answer_jobs_session_created_at
    ON interview_answer_jobs(session_code, created_at DESC);

CREATE INDEX idx_interview_answer_jobs_updated_at
    ON interview_answer_jobs(updated_at DESC);
