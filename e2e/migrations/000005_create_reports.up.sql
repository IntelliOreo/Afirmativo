CREATE TABLE reports (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_code     TEXT NOT NULL UNIQUE REFERENCES sessions(session_code),
    status           TEXT NOT NULL DEFAULT 'generating',  -- generating, ready, failed
    content_en       TEXT,
    content_es       TEXT,
    strengths        JSONB,        -- JSON array of strings
    weaknesses       JSONB,        -- JSON array of strings
    recommendation   TEXT,
    question_count   INT NOT NULL DEFAULT 0,
    duration_minutes INT NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_reports_session_code ON reports(session_code);
