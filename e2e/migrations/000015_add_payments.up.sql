CREATE TABLE payments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    checkout_session_id TEXT UNIQUE,
    session_code        TEXT REFERENCES sessions(session_code) ON DELETE SET NULL,
    amount_cents        INT NOT NULL,
    currency            TEXT NOT NULL,
    status              TEXT NOT NULL,
    reveal_pin          TEXT,
    reveal_expires_at   TIMESTAMPTZ,
    reveal_consumed_at  TIMESTAMPTZ,
    failure_code        TEXT,
    failure_detail      TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_payments_session_code ON payments(session_code);
