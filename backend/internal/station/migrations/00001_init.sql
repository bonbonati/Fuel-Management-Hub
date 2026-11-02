-- +goose Up
-- Station service schema.

CREATE TABLE stations (
    id          text PRIMARY KEY,
    seq         bigint GENERATED ALWAYS AS IDENTITY,
    name        jsonb   NOT NULL,
    area        jsonb   NOT NULL,
    distance_km numeric(6, 1) NOT NULL DEFAULT 0,
    wait        jsonb   NOT NULL DEFAULT '{"ar":"","en":""}',
    phone       text    NOT NULL,
    bank_mask   text    NOT NULL DEFAULT ''
);

CREATE TABLE pumps (
    station_id text    NOT NULL REFERENCES stations (id),
    number     int     NOT NULL CHECK (number > 0),
    grades     text[]  NOT NULL CHECK (cardinality(grades) > 0),
    flow_rate  int     NOT NULL CHECK (flow_rate BETWEEN 10 AND 200),
    online     boolean NOT NULL DEFAULT true,
    PRIMARY KEY (station_id, number)
);

CREATE SEQUENCE staff_seq START 100;
CREATE TABLE staff (
    id         text PRIMARY KEY DEFAULT 'a' || nextval('staff_seq'),
    seq        bigint GENERATED ALWAYS AS IDENTITY,
    station_id text    NOT NULL REFERENCES stations (id),
    name       jsonb   NOT NULL,
    code       text    NOT NULL,
    role       text    NOT NULL CHECK (role IN ('sup', 'att')),
    shift      text    NOT NULL CHECK (shift IN ('A', 'B', 'C')),
    device     text    NOT NULL DEFAULT '',
    active     boolean NOT NULL DEFAULT true,
    CONSTRAINT staff_station_code_key UNIQUE (station_id, code)
);

-- Regulated price per litre (SDG), set nationally.
CREATE TABLE fuel_prices (
    grade text PRIMARY KEY CHECK (grade IN ('91', '95', 'Diesel')),
    price bigint NOT NULL CHECK (price > 0)
);

-- +goose Down
DROP TABLE fuel_prices, staff, pumps, stations;
DROP SEQUENCE staff_seq;
