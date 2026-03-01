-- Migration 000003: Align sessions table with v2 spec, create question_areas + answers.

-- Step 1: Drop old columns from sessions
ALTER TABLE sessions DROP COLUMN IF EXISTS timer_seconds;
ALTER TABLE sessions DROP COLUMN IF EXISTS started_at;

-- Step 2: Add v2 columns to sessions
ALTER TABLE sessions ADD COLUMN interview_budget_seconds INT NOT NULL DEFAULT 2400;
ALTER TABLE sessions ADD COLUMN interview_lapsed_seconds INT NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN interview_lapsed_updated_at TIMESTAMPTZ;
ALTER TABLE sessions ADD COLUMN interview_started_at TIMESTAMPTZ;
ALTER TABLE sessions ADD COLUMN current_interview_started_at TIMESTAMPTZ;
ALTER TABLE sessions ADD COLUMN last_api_call_at TIMESTAMPTZ;
ALTER TABLE sessions ADD COLUMN conversation_history JSONB;

-- Step 3: Add status index (expires_at index already exists from 000001)
CREATE INDEX idx_sessions_status ON sessions(status);

-- Step 4: Create question_areas table
CREATE TABLE question_areas (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_code     TEXT NOT NULL REFERENCES sessions(session_code),
    area             TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'in_progress',
    questions_count  INT NOT NULL DEFAULT 0,
    area_started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    area_ended_at    TIMESTAMPTZ,

    UNIQUE(session_code, area)
);

CREATE INDEX idx_question_areas_session_code ON question_areas(session_code);
CREATE INDEX idx_question_areas_in_progress ON question_areas(session_code) WHERE status = 'in_progress';

-- Step 5: Create answers table
CREATE TABLE answers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_code    TEXT NOT NULL REFERENCES sessions(session_code),
    area            TEXT NOT NULL,
    question_text   TEXT,
    audio_urls      JSONB,
    transcript_es   TEXT,
    transcript_en   TEXT,
    ai_evaluation   JSONB,
    sufficiency     TEXT,
    flags           JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_answers_session_code ON answers(session_code);
CREATE INDEX idx_answers_session_area ON answers(session_code, area);
