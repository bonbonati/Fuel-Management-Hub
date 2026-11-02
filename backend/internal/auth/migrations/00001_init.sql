-- +goose Up
-- Auth service schema. Demo users are created by the service (bcrypt in Go),
-- not by SQL, so no password hash is ever committed to the repository.

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username      text        NOT NULL,
    password_hash text        NOT NULL,
    role          text        NOT NULL CHECK (role IN ('driver', 'fleet_manager', 'attendant', 'station_admin')),
    org_id        text        NOT NULL DEFAULT '',
    station_id    text        NOT NULL DEFAULT '',
    actor_id      text        NOT NULL DEFAULT '',
    name          jsonb       NOT NULL,
    active        boolean     NOT NULL DEFAULT true,
    demo          boolean     NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now()
);
-- Usernames are case-insensitive.
CREATE UNIQUE INDEX users_username_key ON users (lower(username));

-- +goose Down
DROP TABLE users;
