CREATE TABLE coupons (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code          TEXT UNIQUE NOT NULL,
    max_uses      INT NOT NULL,
    current_uses  INT NOT NULL DEFAULT 0,
    discount_pct  INT NOT NULL DEFAULT 100,
    expires_at    TIMESTAMPTZ,
    source        TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
