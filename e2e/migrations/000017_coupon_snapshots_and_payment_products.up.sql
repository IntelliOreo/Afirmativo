ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS coupon_max_uses_at_claim INT,
    ADD COLUMN IF NOT EXISTS coupon_current_uses_at_claim INT;

ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS product_type TEXT,
    ADD COLUMN IF NOT EXISTS coupon_code TEXT;

UPDATE payments
SET product_type = 'direct_session'
WHERE product_type IS NULL;

ALTER TABLE payments
    ALTER COLUMN product_type SET DEFAULT 'direct_session',
    ALTER COLUMN product_type SET NOT NULL;
