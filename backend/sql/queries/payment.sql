-- Queries for payments table.

-- name: CreatePendingPayment :one
INSERT INTO payments (amount_cents, currency, product_type, status)
VALUES ($1, $2, $3, 'pending')
RETURNING *;

-- name: AttachCheckoutSessionID :one
UPDATE payments
SET checkout_session_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: GetPaymentByCheckoutSessionID :one
SELECT * FROM payments WHERE checkout_session_id = $1;

-- name: GetPaymentByCheckoutSessionIDForUpdate :one
SELECT * FROM payments WHERE checkout_session_id = $1 FOR UPDATE;

-- name: GetPaymentByIDForUpdate :one
SELECT * FROM payments WHERE id = $1 FOR UPDATE;

-- name: MarkPaymentFailedByID :one
UPDATE payments
SET checkout_session_id = CASE
        WHEN $2 <> '' THEN COALESCE(checkout_session_id, $2)
        ELSE checkout_session_id
    END,
    status = 'failed',
    failure_code = $3,
    failure_detail = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkPaymentPaidUnprovisionedByID :one
UPDATE payments
SET checkout_session_id = CASE
        WHEN $2 <> '' THEN COALESCE(checkout_session_id, $2)
        ELSE checkout_session_id
    END,
    status = 'paid_unprovisioned',
    failure_code = NULL,
    failure_detail = NULL,
    updated_at = $3
WHERE id = $1
RETURNING *;

-- name: MarkPaymentProvisioned :one
UPDATE payments
SET status = 'provisioned',
    session_code = $2,
    coupon_code = NULL,
    reveal_pin = $3,
    reveal_expires_at = $4,
    reveal_consumed_at = NULL,
    failure_code = NULL,
    failure_detail = NULL,
    updated_at = $5
WHERE id = $1
RETURNING *;

-- name: MarkPaymentProvisionedCouponPack :one
UPDATE payments
SET status = 'provisioned',
    session_code = NULL,
    coupon_code = $2,
    reveal_pin = NULL,
    reveal_expires_at = NULL,
    reveal_consumed_at = NULL,
    failure_code = NULL,
    failure_detail = NULL,
    updated_at = $3
WHERE id = $1
RETURNING *;

-- name: MarkPaymentProvisionFailure :one
UPDATE payments
SET failure_code = $2,
    failure_detail = $3,
    updated_at = now()
WHERE checkout_session_id = $1
  AND status = 'paid_unprovisioned'
RETURNING *;

-- name: ConsumePaymentReveal :one
UPDATE payments
SET reveal_consumed_at = $2,
    updated_at = $2
WHERE id = $1
RETURNING *;
