-- Queries for sessions + coupons tables.
-- Used by sqlc to generate type-safe Go code in internal/sqlgen/.

-- name: ClaimCoupon :one
-- Atomically increment coupon usage. Returns the coupon if valid.
UPDATE coupons SET current_uses = current_uses + 1
WHERE code = $1 AND current_uses < max_uses
AND (expires_at IS NULL OR expires_at > now())
RETURNING *;

-- name: CreateSession :one
INSERT INTO sessions (
    session_code,
    pin_hash,
    coupon_code,
    coupon_max_uses_at_claim,
    coupon_current_uses_at_claim,
    status,
    expires_at,
    interview_budget_seconds
)
VALUES ($1, $2, $3, $4, $5, 'created', $6, $7)
RETURNING *;

-- name: CreatePaidSession :exec
INSERT INTO sessions (session_code, pin_hash, payment_id, status, expires_at, interview_budget_seconds)
VALUES ($1, $2, $3, 'created', $4, $5);

-- name: CreateCoupon :one
INSERT INTO coupons (code, max_uses, source)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetSessionByCode :one
SELECT * FROM sessions WHERE session_code = $1;

-- name: StartSession :one
-- Idempotent session start for reconnects. Sets interview_started_at on first call,
-- resets current_interview_started_at on every call. Accepts created, active, or interviewing.
-- preferred_language is only set once (on first start), then remains locked.
UPDATE sessions
SET status = 'interviewing',
    interview_started_at = COALESCE(interview_started_at, now()),
    current_interview_started_at = now(),
    preferred_language = COALESCE(preferred_language, $2)
WHERE session_code = $1
  AND status IN ('created', 'active', 'interviewing')
RETURNING *;

-- name: CompleteSession :exec
-- Marks an interviewing session as completed with ended_at = now().
UPDATE sessions SET status = 'completed', ended_at = now()
WHERE session_code = $1 AND status = 'interviewing';
