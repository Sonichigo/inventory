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
	// ── Step 1: connect as admin ───────────────────────────────────────────────
	adminDSN := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
		cfg.Server, cfg.Port, cfg.InitUser, cfg.InitPass,
	)
	adminConn, err := openWithRetry(adminDSN, 15, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("admin connect failed: %w", err)
	}
	defer adminConn.Close()

	// ── Step 2: create app database if not exists ──────────────────────────────
	var dbExists bool
	if err = adminConn.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)", cfg.DBName,
	).Scan(&dbExists); err != nil {
		return nil, fmt.Errorf("checking database: %w", err)
	}
	if !dbExists {
		log.Printf("Creating database %q", cfg.DBName)
		if _, err = adminConn.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, cfg.DBName)); err != nil {
			return nil, fmt.Errorf("creating database: %w", err)
		}
	}

	// ── Step 3: create app role if not exists ─────────────────────────────────
	var roleExists bool
	if err = adminConn.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)", cfg.User,
	).Scan(&roleExists); err != nil {
		return nil, fmt.Errorf("checking role: %w", err)
	}
	if !roleExists {
		log.Printf("Creating role %q", cfg.User)
		if _, err = adminConn.Exec(fmt.Sprintf(
			`CREATE ROLE "%s" LOGIN PASSWORD '%s'`, cfg.User, cfg.Password,
		)); err != nil {
			return nil, fmt.Errorf("creating role: %w", err)
		}
	}
	if _, err = adminConn.Exec(fmt.Sprintf(
		`GRANT ALL PRIVILEGES ON DATABASE "%s" TO "%s"`, cfg.DBName, cfg.User,
	)); err != nil {
		return nil, fmt.Errorf("granting privileges: %w", err)
	}

	// ── Step 4: connect as app user ───────────────────────────────────────────
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

	log.Println("Database connected — schema managed by Liquibase")
	return &DB{conn: appConn}, nil
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

func (d *DB) Ping() error { return d.conn.Ping() }

// GetInventoryByLocation joins inventory against suppliers on LOWER() columns.
// BAD state:  no indexes → sequential scan on both tables + hash join
// GOOD state: functional indexes on all join columns → index scans, sub-ms
func (d *DB) GetInventoryByLocation(location string) ([]InventoryItem, error) {
	rows, err := d.conn.Query(
		`SELECT i.id, i.name, i.quantity, i.location, i.unit,
		        COALESCE(s.name, ''),
		        COALESCE(s.lead_days, 0)
		   FROM inventory i
		   LEFT JOIN suppliers s
		          ON LOWER(s.location) = LOWER(i.location)
		         AND LOWER(s.item)     = LOWER(i.name)
		  WHERE LOWER(i.location) = LOWER($1)
		  ORDER BY i.name`,
		location,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []InventoryItem
	for rows.Next() {
		var item InventoryItem
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Quantity,
			&item.Location, &item.Unit,
			&item.Supplier, &item.LeadDays,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (d *DB) GetAllInventory() ([]InventoryItem, error) {
	rows, err := d.conn.Query(
		`SELECT id, name, quantity, location, unit FROM inventory ORDER BY location, name LIMIT 100`,
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
	return &item, err
}

func (d *DB) UpdateQuantity(req UpdateQuantityRequest) (*InventoryItem, error) {
	var item InventoryItem
	err := d.conn.QueryRow(
		`UPDATE inventory SET quantity=$1 WHERE id=$2
		 RETURNING id, name, quantity, location, unit`,
		req.Quantity, req.ID,
	).Scan(&item.ID, &item.Name, &item.Quantity, &item.Location, &item.Unit)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("item with id %d not found", req.ID)
	}
	return &item, err
}

func (d *DB) DeleteItem(id int) error {
	res, err := d.conn.Exec(`DELETE FROM inventory WHERE id=$1`, id)
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

// GetLowStock returns items below a quantity threshold across all locations.
// Triggers a full sequential scan + filter in bad state.
// In good state, index on location helps narrow the scan.
func (d *DB) GetLowStock(threshold int) ([]InventoryItem, error) {
	rows, err := d.conn.Query(
		`SELECT i.id, i.name, i.quantity, i.location, i.unit,
		        COALESCE(s.name, ''),
		        COALESCE(s.lead_days, 0)
		   FROM inventory i
		   LEFT JOIN suppliers s
		          ON LOWER(s.location) = LOWER(i.location)
		         AND LOWER(s.item)     = LOWER(i.name)
		  WHERE i.quantity < $1
		  ORDER BY i.quantity ASC, i.location`,
		threshold,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []InventoryItem
	for rows.Next() {
		var item InventoryItem
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Quantity,
			&item.Location, &item.Unit,
			&item.Supplier, &item.LeadDays,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetSupplierSummary returns aggregated supplier stats for a location.
// Involves GROUP BY + AVG + COUNT + JOIN — expensive without indexes.
func (d *DB) GetSupplierSummary(location string) ([]SupplierSummary, error) {
	rows, err := d.conn.Query(
		`SELECT s.name,
		        s.location,
		        COUNT(DISTINCT i.name)    AS item_count,
		        AVG(s.lead_days)          AS avg_lead_days,
		        COALESCE(SUM(i.quantity), 0) AS total_stock
		   FROM suppliers s
		   LEFT JOIN inventory i
		          ON LOWER(i.location) = LOWER(s.location)
		         AND LOWER(i.name)     = LOWER(s.item)
		  WHERE LOWER(s.location) = LOWER($1)
		  GROUP BY s.name, s.location
		  ORDER BY total_stock DESC`,
		location,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []SupplierSummary
	for rows.Next() {
		var s SupplierSummary
		if err := rows.Scan(
			&s.Supplier, &s.Location,
			&s.ItemCount, &s.AvgLeadDays, &s.TotalStock,
		); err != nil {
			return nil, err
		}
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}
