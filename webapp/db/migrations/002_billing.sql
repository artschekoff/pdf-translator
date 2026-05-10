-- +goose Up

-- Plans mirror products/prices created in the payments-service.
-- payments_price_id is the UUID of the BillingPrice in the payments-service.
CREATE TABLE billing_plans (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT NOT NULL,
    pages               INTEGER NOT NULL,
    price_cents         INTEGER NOT NULL,
    payments_price_id   UUID,
    is_active           BOOLEAN NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE user_balances (
    user_id     TEXT PRIMARY KEY,
    pages       INTEGER NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Tracks purchases credited so we don't double-credit on re-sync.
CREATE TABLE credited_purchases (
    purchase_id TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL,
    pages       INTEGER NOT NULL,
    credited_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed starter plans (set payments_price_id after creating prices in payments-service).
INSERT INTO billing_plans (name, pages, price_cents) VALUES
    ('Starter',  50,   900),
    ('Basic',    200,  2900),
    ('Pro',      500,  5900),
    ('Business', 1500, 14900);

-- +goose Down

DROP TABLE IF EXISTS credited_purchases;
DROP TABLE IF EXISTS user_balances;
DROP TABLE IF EXISTS billing_plans;
