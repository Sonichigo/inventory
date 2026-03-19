-- ── schema.sql ────────────────────────────────────────────────────────────────
-- Creates tables only. Safe to run against both fresh and existing databases.
-- No indexes or constraints here — owned by seed files to handle schema drift.

CREATE TABLE IF NOT EXISTS locations (
    id   SERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    city TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS inventory (
    id       SERIAL PRIMARY KEY,
    name     TEXT    NOT NULL,
    quantity INTEGER NOT NULL DEFAULT 0,
    unit     TEXT    NOT NULL DEFAULT 'lbs',
    location TEXT    NOT NULL REFERENCES locations(name) ON UPDATE CASCADE
);

-- suppliers table: joined against inventory in GetInventoryByLocation.
-- No indexes intentionally in bad state — added in good state via seed-good.sql.
CREATE TABLE IF NOT EXISTS suppliers (
    id        SERIAL PRIMARY KEY,
    name      TEXT    NOT NULL,
    location  TEXT    NOT NULL,
    item      TEXT    NOT NULL,
    lead_days INTEGER NOT NULL DEFAULT 1
);

INSERT INTO locations (name, city) VALUES
    ('Seattle','Seattle'),('Portland','Portland'),
    ('San Francisco','San Francisco'),('Austin','Austin'),
    ('Nashville','Nashville'),('Chicago','Chicago'),
    ('Dallas','Dallas'),('Miami','Miami'),
    ('Denver','Denver'),('Phoenix','Phoenix')
ON CONFLICT (name) DO NOTHING;