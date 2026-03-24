CREATE TABLE sessions (
    session_code   TEXT PRIMARY KEY,
    pin_hash       TEXT NOT NULL,
    track          TEXT,
    status         TEXT NOT NULL DEFAULT 'created',
    role           TEXT NOT NULL DEFAULT 'client',
    timer_seconds  INT  NOT NULL DEFAULT 3600,
    started_at     TIMESTAMPTZ,
    ended_at       TIMESTAMPTZ,
    payment_id     TEXT,
    coupon_code    TEXT,
    expires_at     TIMESTAMPTZ NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);
