-- ── seed-bad.sql ──────────────────────────────────────────────────────────────
-- BAD performance state:
--   1. No index on inventory.location
--   2. Query uses LOWER(location) = LOWER($1) — prevents any index use
--   3. Large dataset amplifies the sequential scan cost
--
-- At 60 req/sec this creates measurable wait time visible in DBMarlin.
-- To demonstrate the problem, apply this configmap and restart the deployment.

-- Drop the index if it exists from a previous "fix" demo
DROP INDEX IF EXISTS idx_inventory_location;
DROP INDEX IF EXISTS idx_inventory_location_lower;

-- Seed inventory across all locations
INSERT INTO inventory (name, quantity, unit, location) VALUES
    ('Brisket',           50,  'lbs',    'Seattle'),
    ('Pulled Pork',       30,  'lbs',    'Seattle'),
    ('Baby Back Ribs',    20,  'racks',  'Seattle'),
    ('Sausage Links',     60,  'links',  'Seattle'),
    ('Chicken Wings',    100,  'pieces', 'Seattle'),
    ('Smoked Turkey',     15,  'lbs',    'Seattle'),
    ('Burnt Ends',        25,  'lbs',    'Seattle'),
    ('Pork Belly',        18,  'lbs',    'Seattle'),
    ('Brisket',           40,  'lbs',    'Portland'),
    ('Sausage Links',     60,  'links',  'Portland'),
    ('Pulled Pork',       35,  'lbs',    'Portland'),
    ('Baby Back Ribs',    22,  'racks',  'Portland'),
    ('Chicken Wings',     80,  'pieces', 'Portland'),
    ('Smoked Salmon',     12,  'lbs',    'Portland'),
    ('Brisket',           45,  'lbs',    'Austin'),
    ('Jalapeño Sausage',  35,  'links',  'Austin'),
    ('Pulled Pork',       28,  'lbs',    'Austin'),
    ('Beef Ribs',         16,  'racks',  'Austin'),
    ('Chicken Wings',     90,  'pieces', 'Austin'),
    ('Smoked Turkey',     20,  'lbs',    'Austin'),
    ('Pulled Pork',       25,  'lbs',    'Nashville'),
    ('Smoked Turkey',     15,  'lbs',    'Nashville'),
    ('Baby Back Ribs',    18,  'racks',  'Nashville'),
    ('Brisket',           38,  'lbs',    'Nashville'),
    ('Hot Chicken',       50,  'pieces', 'Nashville'),
    ('Chicken Wings',    100,  'pieces', 'San Francisco'),
    ('Pulled Pork',       20,  'lbs',    'San Francisco'),
    ('Brisket',           30,  'lbs',    'San Francisco'),
    ('Sausage Links',     40,  'links',  'San Francisco'),
    ('Brisket',           55,  'lbs',    'Chicago'),
    ('Italian Sausage',   70,  'links',  'Chicago'),
    ('Pulled Pork',       32,  'lbs',    'Chicago'),
    ('Baby Back Ribs',    24,  'racks',  'Chicago'),
    ('Chicken Wings',     95,  'pieces', 'Chicago'),
    ('Brisket',           48,  'lbs',    'Dallas'),
    ('Jalapeño Sausage',  40,  'links',  'Dallas'),
    ('Beef Ribs',         20,  'racks',  'Dallas'),
    ('Pulled Pork',       30,  'lbs',    'Dallas'),
    ('Chicken Wings',     85,  'pieces', 'Dallas'),
    ('Brisket',           42,  'lbs',    'Miami'),
    ('Pulled Pork',       28,  'lbs',    'Miami'),
    ('Chicken Wings',     75,  'pieces', 'Miami'),
    ('Smoked Turkey',     18,  'lbs',    'Miami'),
    ('Brisket',           50,  'lbs',    'Denver'),
    ('Pulled Pork',       26,  'lbs',    'Denver'),
    ('Baby Back Ribs',    20,  'racks',  'Denver'),
    ('Smoked Turkey',     14,  'lbs',    'Denver'),
    ('Brisket',           44,  'lbs',    'Phoenix'),
    ('Pulled Pork',       24,  'lbs',    'Phoenix'),
    ('Jalapeño Sausage',  38,  'links',  'Phoenix'),
    ('Chicken Wings',     88,  'pieces', 'Phoenix')
ON CONFLICT (name, location) DO NOTHING;