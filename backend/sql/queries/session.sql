-- Queries for sessions + coupons tables.
-- Used by sqlc to generate type-safe Go code in internal/sqlgen/.

-- name: ClaimCoupon :one
-- Atomically increment coupon usage. Returns the coupon if valid.
UPDATE coupons SET current_uses = current_uses + 1
WHERE code = $1 AND current_uses < max_uses
AND (expires_at IS NULL OR expires_at > now())
RETURNING *;

-- name: CreateSession :one
INSERT INTO sessions (session_code, pin_hash, coupon_code, status, expires_at)
VALUES ($1, $2, $3, 'created', $4)
RETURNING *;

-- name: GetSessionByCode :one
SELECT * FROM sessions WHERE session_code = $1;

-- name: StartSession :one
-- Idempotent session start for reconnects. Sets interview_started_at on first call,
-- resets current_interview_started_at on every call. Accepts created, active, or interviewing.
UPDATE sessions
SET status = 'interviewing',
    interview_started_at = COALESCE(interview_started_at, now()),
    current_interview_started_at = now()
WHERE session_code = $1
  AND status IN ('created', 'active', 'interviewing')
RETURNING *;
