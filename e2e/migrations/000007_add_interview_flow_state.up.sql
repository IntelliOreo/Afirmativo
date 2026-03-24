-- Migration 000007: Add explicit interview flow state and non-criterion event audit.

ALTER TABLE sessions ADD COLUMN flow_step TEXT NOT NULL DEFAULT 'disclaimer';
ALTER TABLE sessions ADD COLUMN flow_version INT NOT NULL DEFAULT 2;
ALTER TABLE sessions ADD COLUMN expected_turn_id TEXT;
ALTER TABLE sessions ADD COLUMN display_question_number INT NOT NULL DEFAULT 1;

ALTER TABLE sessions
    ADD CONSTRAINT chk_sessions_flow_step
    CHECK (flow_step IN ('disclaimer', 'readiness', 'criterion', 'done'));

ALTER TABLE sessions
    ADD CONSTRAINT chk_sessions_display_question_number
    CHECK (display_question_number > 0);

CREATE INDEX idx_sessions_flow_step ON sessions(flow_step);

CREATE TABLE interview_events (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_code TEXT NOT NULL REFERENCES sessions(session_code) ON DELETE CASCADE,
    event_type   TEXT NOT NULL,
    answer_text  TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_interview_events_type
        CHECK (event_type IN ('disclaimer_ack', 'readiness_ack'))
);

CREATE INDEX idx_interview_events_session_code ON interview_events(session_code);
CREATE INDEX idx_interview_events_session_type ON interview_events(session_code, event_type);
