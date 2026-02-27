-- Queries for payments table.
-- Table not yet migrated — queries will be uncommented after migration.

-- name: CreatePayment :one
-- INSERT INTO payments (session_code, stripe_id, status, amount_cents)
-- VALUES ($1, $2, $3, $4)
-- RETURNING *;

-- name: GetPaymentByStripeID :one
-- SELECT * FROM payments WHERE stripe_id = $1;
