-- +goose Up
-- Fueling service schema: transactions, odometer checks, security log, outbox.

CREATE SEQUENCE tx_seq START 90413;
CREATE TABLE transactions (
    id              text PRIMARY KEY,
    org_id          text        NOT NULL DEFAULT '',  -- '' = individual customer
    vehicle_id      text        NOT NULL DEFAULT '',
    driver_id       text        NOT NULL DEFAULT '',
    plate           jsonb       NOT NULL,
    station_id      text        NOT NULL,
    pump            int         NOT NULL CHECK (pump > 0),
    fuel            text        NOT NULL CHECK (fuel IN ('91', '95', 'Diesel')),
    amount          bigint      NOT NULL CHECK (amount > 0),
    price_per_litre bigint      NOT NULL CHECK (price_per_litre > 0),
    status          text        NOT NULL CHECK (status IN ('auth', 'fueling', 'done')),
    code            text        NOT NULL DEFAULT '',
    source          text        NOT NULL CHECK (source IN ('app', 'qr', 'rfid')),
    odometer_km     int,
    created_at      timestamptz NOT NULL DEFAULT now(),
    started_at      timestamptz,
    completed_at    timestamptz
);
CREATE INDEX transactions_driver_idx  ON transactions (driver_id, created_at DESC);
CREATE INDEX transactions_org_idx     ON transactions (org_id, created_at DESC);
CREATE INDEX transactions_station_idx ON transactions (station_id, created_at DESC);
-- A start code identifies one open authorization per station.
CREATE UNIQUE INDEX transactions_open_code_key ON transactions (station_id, code) WHERE status = 'auth' AND code <> '';

CREATE TABLE odometer_readings (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    driver_id   text        NOT NULL,
    km          int         NOT NULL CHECK (km >= 0),
    recorded_at timestamptz NOT NULL DEFAULT now(),
    consumed_by text        -- transaction that used this reading
);
CREATE INDEX odometer_pending_idx ON odometer_readings (driver_id, recorded_at DESC) WHERE consumed_by IS NULL;

CREATE TABLE security_events (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id     text        NOT NULL,
    severity   text        NOT NULL CHECK (severity IN ('critical', 'warning')),
    title      jsonb       NOT NULL,
    detail     jsonb       NOT NULL,
    ref        text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX security_events_org_idx ON security_events (org_id, created_at DESC);

-- Transactional outbox (see internal/platform/events).
CREATE TABLE outbox (
    id           uuid PRIMARY KEY,
    type         text        NOT NULL,
    source       text        NOT NULL,
    occurred_at  timestamptz NOT NULL,
    payload      jsonb       NOT NULL,
    published_at timestamptz
);
CREATE INDEX outbox_pending_idx ON outbox (occurred_at) WHERE published_at IS NULL;

-- +goose Down
DROP TABLE outbox, security_events, odometer_readings, transactions;
DROP SEQUENCE tx_seq;
