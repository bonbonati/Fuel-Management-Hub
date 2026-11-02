-- +goose Up
-- Payment service schema: wallets + append-only ledger, VAT, settlements.

CREATE TABLE wallets (
    org_id     text PRIMARY KEY,
    balance    bigint      NOT NULL CHECK (balance >= 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Append-only ledger. Usage of a vehicle in a month is derived from it.
CREATE TABLE ledger (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id     text        NOT NULL REFERENCES wallets (org_id),
    vehicle_id text,
    tx_id      text,
    kind       text        NOT NULL CHECK (kind IN ('opening_usage', 'charge', 'topup', 'refund')),
    amount     bigint      NOT NULL CHECK (amount > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);
-- One charge and at most one refund per fueling transaction (idempotency).
CREATE UNIQUE INDEX ledger_kind_tx_key ON ledger (kind, tx_id) WHERE tx_id IS NOT NULL;
CREATE INDEX ledger_usage_idx ON ledger (org_id, vehicle_id, created_at);

-- Single-row VAT configuration.
CREATE TABLE tax_settings (
    id      int PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled boolean      NOT NULL,
    rate    numeric(5, 2) NOT NULL CHECK (rate BETWEEN 0 AND 100)
);
INSERT INTO tax_settings (id, enabled, rate) VALUES (1, true, 17);

CREATE TABLE settlements (
    station_id  text   NOT NULL,
    payout_date date   NOT NULL,
    amount      bigint NOT NULL DEFAULT 0,
    fee         bigint NOT NULL DEFAULT 0,
    status      text   NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'paid')),
    PRIMARY KEY (station_id, payout_date)
);

-- Event ids already handled by consumers (deduplication for at-least-once delivery).
CREATE TABLE processed_events (
    id           uuid PRIMARY KEY,
    processed_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE processed_events, settlements, tax_settings, ledger, wallets;
