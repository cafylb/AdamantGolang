CREATE TABLE IF NOT EXISTS users (
    tg_id BIGINT PRIMARY KEY,
    stars_spend INTEGER NOT NULL DEFAULT 0,
    balance BIGINT NOT NULL DEFAULT 0
);

DROP TABLE IF EXISTS purchases CASCADE;

CREATE TABLE purchases (
    id BIGSERIAL PRIMARY KEY,
    tg_id BIGINT NOT NULL REFERENCES users(tg_id) ON DELETE CASCADE,
    data TEXT NOT NULL,
    complete BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS core (
    id INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    stars_bought INTEGER NOT NULL DEFAULT 0,
    came_by_referal INTEGER NOT NULL DEFAULT 0,
    completed_orders INTEGER NOT NULL DEFAULT 0,
    via_adamant_stars INTEGER NOT NULL DEFAULT 0,
    via_ton_stars INTEGER NOT NULL DEFAULT 0,
    via_cryptomus_stars INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS ix_purchases_tg_id ON purchases(tg_id);
CREATE INDEX IF NOT EXISTS ix_purchases_complete_created_at ON purchases(complete, created_at);

INSERT INTO core (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;
