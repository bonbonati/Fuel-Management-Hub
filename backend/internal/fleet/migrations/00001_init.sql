-- +goose Up
-- Fleet service schema. Bilingual values ({"ar","en"}) and plates are JSONB.
-- Text ids keep the prototype's readable formats (t1, d3, f6, R-1044).

CREATE TABLE orgs (
    id    text PRIMARY KEY,
    name  jsonb NOT NULL,
    phone text  NOT NULL,
    city  jsonb NOT NULL
);

CREATE SEQUENCE tier_seq START 100;
CREATE TABLE tiers (
    id           text PRIMARY KEY DEFAULT 't' || nextval('tier_seq'),
    seq          bigint GENERATED ALWAYS AS IDENTITY,  -- stable display order
    org_id       text   NOT NULL REFERENCES orgs (id),
    icon         text   NOT NULL DEFAULT 'layers',
    color        text   NOT NULL DEFAULT 'bl',
    name         jsonb  NOT NULL,
    budget       bigint NOT NULL CHECK (budget >= 0),
    daily_litres int    NOT NULL CHECK (daily_litres > 0),
    fuel_window  text   NOT NULL
);
CREATE INDEX tiers_org_idx ON tiers (org_id, seq);

CREATE SEQUENCE driver_seq START 100;
CREATE TABLE drivers (
    id            text PRIMARY KEY DEFAULT 'd' || nextval('driver_seq'),
    seq           bigint GENERATED ALWAYS AS IDENTITY,
    org_id        text    NOT NULL REFERENCES orgs (id),
    name          jsonb   NOT NULL,
    phone         text    NOT NULL,
    code          text    NOT NULL,
    app_activated boolean NOT NULL DEFAULT false,
    active        boolean NOT NULL DEFAULT true,
    CONSTRAINT drivers_org_code_key UNIQUE (org_id, code)
);
CREATE INDEX drivers_org_idx ON drivers (org_id, seq);

CREATE SEQUENCE vehicle_seq START 100;
CREATE TABLE vehicles (
    id           text PRIMARY KEY DEFAULT 'f' || nextval('vehicle_seq'),
    seq          bigint GENERATED ALWAYS AS IDENTITY,
    org_id       text    NOT NULL REFERENCES orgs (id),
    plate        jsonb   NOT NULL,
    name         jsonb   NOT NULL,
    driver_id    text    REFERENCES drivers (id) ON DELETE SET NULL,
    fuel         text    NOT NULL CHECK (fuel IN ('91', '95', 'Diesel')),
    tier_id      text    REFERENCES tiers (id) ON DELETE SET NULL,
    custom_limit bigint  CHECK (custom_limit >= 0),
    extra        bigint  NOT NULL DEFAULT 0 CHECK (extra >= 0),
    active       boolean NOT NULL DEFAULT true,
    tag_battery  int     NOT NULL DEFAULT 100 CHECK (tag_battery BETWEEN 0 AND 100)
);
CREATE INDEX vehicles_org_idx ON vehicles (org_id, seq);
-- A driver drives at most one vehicle.
CREATE UNIQUE INDEX vehicles_driver_key ON vehicles (driver_id) WHERE driver_id IS NOT NULL;

CREATE TABLE rules (
    org_id              text PRIMARY KEY REFERENCES orgs (id),
    network             boolean NOT NULL,
    anti_siphon         boolean NOT NULL,
    geofence            boolean NOT NULL,
    wrong_fuel          boolean NOT NULL,
    odometer            boolean NOT NULL,
    max_fill            boolean NOT NULL,
    max_fill_amount     bigint  NOT NULL CHECK (max_fill_amount >= 0),
    hours               boolean NOT NULL,
    network_station_ids text[]  NOT NULL DEFAULT '{}'
);

CREATE SEQUENCE request_seq START 1044;
CREATE TABLE requests (
    id         text PRIMARY KEY DEFAULT 'R-' || nextval('request_seq'),
    org_id     text        NOT NULL REFERENCES orgs (id),
    driver_id  text        NOT NULL REFERENCES drivers (id),
    vehicle_id text        REFERENCES vehicles (id) ON DELETE SET NULL,
    type       text        NOT NULL CHECK (type IN ('limit', 'activate')),
    amount     bigint      NOT NULL DEFAULT 0,
    reason     text        NOT NULL,
    note       jsonb       NOT NULL DEFAULT '{"ar":"","en":""}',
    status     text        NOT NULL CHECK (status IN ('pending', 'approved', 'rejected')),
    approved   bigint      NOT NULL DEFAULT 0,
    reply      jsonb       NOT NULL DEFAULT '{"ar":"","en":""}',
    created_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz
);
CREATE INDEX requests_org_idx ON requests (org_id, created_at DESC);

-- +goose Down
DROP TABLE requests, rules, vehicles, drivers, tiers, orgs;
DROP SEQUENCE request_seq, vehicle_seq, driver_seq, tier_seq;
