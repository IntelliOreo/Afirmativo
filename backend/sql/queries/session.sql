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
