package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
)

type Config struct {
	InitUser string
	InitPass string
	Server   string
	Port     string
	User     string
	Password string
	DBName   string
}

type DB struct {
	conn *sql.DB
}

func NewDB(cfg Config) (*DB, error) {
	// ── Step 1: connect as admin to postgres maintenance DB ───────────────────
	adminDSN := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
		cfg.Server, cfg.Port, cfg.InitUser, cfg.InitPass,
	)
	adminConn, err := openWithRetry(adminDSN, 15, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("admin connect failed: %w", err)
	}
	defer adminConn.Close()

	// ── Step 2: create app database if not exists ─────────────────────────────
	var dbExists bool
	err = adminConn.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)", cfg.DBName,
	).Scan(&dbExists)
	if err != nil {
		return nil, fmt.Errorf("checking database existence: %w", err)
	}
	if !dbExists {
		log.Printf("Creating database %q", cfg.DBName)
		_, err = adminConn.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, cfg.DBName))
		if err != nil {
			return nil, fmt.Errorf("creating database: %w", err)
		}
	}

	// ── Step 3: create app role if not exists ─────────────────────────────────
	var roleExists bool
	err = adminConn.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)", cfg.User,
	).Scan(&roleExists)
	if err != nil {
		return nil, fmt.Errorf("checking role existence: %w", err)
	}
	if !roleExists {
		log.Printf("Creating role %q", cfg.User)
		_, err = adminConn.Exec(
			fmt.Sprintf(`CREATE ROLE "%s" LOGIN PASSWORD '%s'`, cfg.User, cfg.Password),
		)
		if err != nil {
			return nil, fmt.Errorf("creating role: %w", err)
		}
	}

	_, err = adminConn.Exec(
		fmt.Sprintf(`GRANT ALL PRIVILEGES ON DATABASE "%s" TO "%s"`, cfg.DBName, cfg.User),
	)
	if err != nil {
		return nil, fmt.Errorf("granting privileges: %w", err)
	}

	// ── Step 4: reconnect as app user ─────────────────────────────────────────
	appDSN := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.Server, cfg.Port, cfg.User, cfg.Password, cfg.DBName,
	)
	appConn, err := openWithRetry(appDSN, 10, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("app connect failed: %w", err)
	}

	appConn.SetMaxOpenConns(25)
	appConn.SetMaxIdleConns(5)
	appConn.SetConnMaxLifetime(5 * time.Minute)

	database := &DB{conn: appConn}

	// ── Step 5: run schema migrations ─────────────────────────────────────────
	if err := database.migrate(); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	log.Println("Database initialised successfully")
	return database, nil
}

func openWithRetry(dsn string, attempts int, delay time.Duration) (*sql.DB, error) {
	var conn *sql.DB
	var err error
	for i := 1; i <= attempts; i++ {
		conn, err = sql.Open("postgres", dsn)
		if err == nil {
			if err = conn.Ping(); err == nil {
				return conn, nil
			}
			conn.Close()
		}
		log.Printf("DB not ready (attempt %d/%d): %v — retrying in %s", i, attempts, err, delay)
		time.Sleep(delay)
	}
	return nil, fmt.Errorf("could not connect after %d attempts: %w", attempts, err)
}

func (d *DB) migrate() error {
	schema := `
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

	INSERT INTO locations (name, city) VALUES
		('Seattle',      'Seattle'),
		('Portland',     'Portland'),
		('San Francisco','San Francisco'),
		('Austin',       'Austin'),
		('Nashville',    'Nashville')
	ON CONFLICT (name) DO NOTHING;

	INSERT INTO inventory (name, quantity, unit, location) VALUES
		('Brisket',          50,  'lbs',    'Seattle'),
		('Pulled Pork',      30,  'lbs',    'Seattle'),
		('Baby Back Ribs',   20,  'racks',  'Seattle'),
		('Brisket',          40,  'lbs',    'Portland'),
		('Sausage Links',    60,  'links',  'Portland'),
		('Chicken Wings',   100,  'pieces', 'San Francisco'),
		('Brisket',          45,  'lbs',    'Austin'),
		('Jalapeño Sausage', 35,  'links',  'Austin'),
		('Pulled Pork',      25,  'lbs',    'Nashville'),
		('Smoked Turkey',    15,  'lbs',    'Nashville')
	ON CONFLICT DO NOTHING;
	`
	_, err := d.conn.Exec(schema)
	return err
}

func (d *DB) Ping() error {
	return d.conn.Ping()
}

func (d *DB) GetInventoryByLocation(location string) ([]InventoryItem, error) {
	rows, err := d.conn.Query(
		`SELECT id, name, quantity, location, unit
		   FROM inventory
		  WHERE LOWER(location) = LOWER($1)
		  ORDER BY name`,
		location,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []InventoryItem
	for rows.Next() {
		var item InventoryItem
		if err := rows.Scan(&item.ID, &item.Name, &item.Quantity, &item.Location, &item.Unit); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (d *DB) GetAllInventory() ([]InventoryItem, error) {
	rows, err := d.conn.Query(
		`SELECT id, name, quantity, location, unit FROM inventory ORDER BY location, name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []InventoryItem
	for rows.Next() {
		var item InventoryItem
		if err := rows.Scan(&item.ID, &item.Name, &item.Quantity, &item.Location, &item.Unit); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (d *DB) AddItem(req AddItemRequest) (*InventoryItem, error) {
	var item InventoryItem
	err := d.conn.QueryRow(
		`INSERT INTO inventory (name, quantity, location, unit)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, name, quantity, location, unit`,
		req.Name, req.Quantity, req.Location, req.Unit,
	).Scan(&item.ID, &item.Name, &item.Quantity, &item.Location, &item.Unit)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *DB) UpdateQuantity(req UpdateQuantityRequest) (*InventoryItem, error) {
	var item InventoryItem
	err := d.conn.QueryRow(
		`UPDATE inventory
		    SET quantity = $1
		  WHERE id = $2
		  RETURNING id, name, quantity, location, unit`,
		req.Quantity, req.ID,
	).Scan(&item.ID, &item.Name, &item.Quantity, &item.Location, &item.Unit)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("item with id %d not found", req.ID)
	}
	return &item, err
}

func (d *DB) DeleteItem(id int) error {
	res, err := d.conn.Exec(`DELETE FROM inventory WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("item with id %d not found", id)
	}
	return nil
}

func (d *DB) GetLocations() ([]Location, error) {
	rows, err := d.conn.Query(`SELECT id, name, city FROM locations ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var locs []Location
	for rows.Next() {
		var l Location
		if err := rows.Scan(&l.ID, &l.Name, &l.City); err != nil {
			return nil, err
		}
		locs = append(locs, l)
	}
	return locs, rows.Err()
}