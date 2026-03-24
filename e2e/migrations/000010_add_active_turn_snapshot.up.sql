-- Migration 000010: Persist the currently issued interview turn and its answer deadlines.

ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS active_question_text_es TEXT,
    ADD COLUMN IF NOT EXISTS active_question_text_en TEXT,
    ADD COLUMN IF NOT EXISTS active_question_area TEXT,
    ADD COLUMN IF NOT EXISTS active_question_kind TEXT,
    ADD COLUMN IF NOT EXISTS active_question_issued_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS active_answer_deadline_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS active_answer_buffer_deadline_at TIMESTAMPTZ;

ALTER TABLE sessions
    ADD CONSTRAINT chk_sessions_active_question_kind
    CHECK (
        active_question_kind IS NULL OR
        active_question_kind IN ('disclaimer', 'readiness', 'criterion')
    );
