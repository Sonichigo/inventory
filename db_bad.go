// +build bad

package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
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
	// BAD: Severely limited connection pool → connection exhaustion under load
	appDSN := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.Server, cfg.Port, cfg.User, cfg.Password, cfg.DBName,
	)
	appConn, err := openWithRetry(appDSN, 10, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("app connect failed: %w", err)
	}
	// BAD: Only 2 max connections instead of 25
	appConn.SetMaxOpenConns(2)
	appConn.SetMaxIdleConns(1)
	appConn.SetConnMaxLifetime(1 * time.Minute)

	log.Println("Database connected [BAD VERSION] — schema managed by Liquibase")
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

// GetInventoryByLocation — BAD VERSION
// Anti-pattern: N+1 queries instead of JOIN
// 1. Fetch all inventory for location (no JOIN)
// 2. For EACH item, make a separate query to suppliers table
// With 50k rows, this makes 50k+ queries instead of 1 JOIN
func (d *DB) GetInventoryByLocation(location string) ([]InventoryItem, error) {
	// BAD: No JOIN, fetch inventory only
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
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Quantity,
			&item.Location, &item.Unit,
		); err != nil {
			return nil, err
		}

		// BAD: N+1 query pattern — one query PER item to get supplier
		var supplierName sql.NullString
		var leadDays sql.NullInt64
		supplierErr := d.conn.QueryRow(
			`SELECT name, lead_days FROM suppliers
			  WHERE LOWER(location) = LOWER($1)
			    AND LOWER(item) = LOWER($2)
			  LIMIT 1`,
			item.Location, item.Name,
		).Scan(&supplierName, &leadDays)

		if supplierErr == nil {
			item.Supplier = supplierName.String
			item.LeadDays = int(leadDays.Int64)
		}
		// Ignore errors, just use empty supplier

		items = append(items, item)
	}
	return items, rows.Err()
}

// GetAllInventory — BAD VERSION
// Anti-pattern: No LIMIT, loads ALL 50k+ rows into memory
func (d *DB) GetAllInventory() ([]InventoryItem, error) {
	// BAD: Removed LIMIT 100 — now fetches all 50k+ rows
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

// GetLowStock — BAD VERSION
// Anti-pattern 1: Fetch ALL inventory (no threshold filter in SQL)
// Anti-pattern 2: Filter in Go memory instead of WHERE clause
// Anti-pattern 3: N+1 queries for suppliers
func (d *DB) GetLowStock(threshold int) ([]InventoryItem, error) {
	// BAD: Fetch everything, no WHERE clause
	rows, err := d.conn.Query(
		`SELECT id, name, quantity, location, unit
		   FROM inventory
		  ORDER BY quantity ASC, location`,
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
		); err != nil {
			return nil, err
		}

		// BAD: Filter in Go instead of SQL
		if item.Quantity >= threshold {
			continue
		}

		// BAD: N+1 query for supplier
		var supplierName sql.NullString
		var leadDays sql.NullInt64
		supplierErr := d.conn.QueryRow(
			`SELECT name, lead_days FROM suppliers
			  WHERE LOWER(location) = LOWER($1)
			    AND LOWER(item) = LOWER($2)
			  LIMIT 1`,
			item.Location, item.Name,
		).Scan(&supplierName, &leadDays)

		if supplierErr == nil {
			item.Supplier = supplierName.String
			item.LeadDays = int(leadDays.Int64)
		}

		items = append(items, item)
	}
	return items, rows.Err()
}

// GetSupplierSummary — BAD VERSION
// Anti-pattern 1: Fetch all suppliers (ignoring location filter)
// Anti-pattern 2: Manual aggregation in Go instead of GROUP BY
// Anti-pattern 3: Nested loops to join with inventory
func (d *DB) GetSupplierSummary(location string) ([]SupplierSummary, error) {
	// BAD: Fetch ALL suppliers, no WHERE clause
	suppRows, err := d.conn.Query(
		`SELECT name, location, item, lead_days FROM suppliers`,
	)
	if err != nil {
		return nil, err
	}
	defer suppRows.Close()

	type supplier struct {
		name      string
		location  string
		item      string
		leadDays  int
	}

	var allSuppliers []supplier
	for suppRows.Next() {
		var s supplier
		if err := suppRows.Scan(&s.name, &s.location, &s.item, &s.leadDays); err != nil {
			return nil, err
		}
		// BAD: Filter in Go instead of SQL WHERE
		if strings.EqualFold(s.location, location) {
			allSuppliers = append(allSuppliers, s)
		}
	}

	// BAD: Fetch ALL inventory (no JOIN, no WHERE)
	invRows, err := d.conn.Query(
		`SELECT name, location, quantity FROM inventory`,
	)
	if err != nil {
		return nil, err
	}
	defer invRows.Close()

	type invItem struct {
		name     string
		location string
		quantity int
	}

	var allInventory []invItem
	for invRows.Next() {
		var i invItem
		if err := invRows.Scan(&i.name, &i.location, &i.quantity); err != nil {
			return nil, err
		}
		allInventory = append(allInventory, i)
	}

	// BAD: Manual aggregation in Go with nested loops (O(N*M) complexity)
	summaryMap := make(map[string]*SupplierSummary)
	for _, s := range allSuppliers {
		key := s.name + "|" + s.location

		if _, exists := summaryMap[key]; !exists {
			summaryMap[key] = &SupplierSummary{
				Supplier:    s.name,
				Location:    s.location,
				ItemCount:   0,
				AvgLeadDays: 0,
				TotalStock:  0,
			}
		}

		summary := summaryMap[key]
		summary.ItemCount++
		summary.AvgLeadDays += float64(s.leadDays)

		// BAD: Nested loop to find matching inventory
		for _, inv := range allInventory {
			if strings.EqualFold(inv.location, s.location) &&
			   strings.EqualFold(inv.name, s.item) {
				summary.TotalStock += inv.quantity
			}
		}
	}

	// Calculate averages
	var summaries []SupplierSummary
	for _, s := range summaryMap {
		if s.ItemCount > 0 {
			s.AvgLeadDays = s.AvgLeadDays / float64(s.ItemCount)
		}
		summaries = append(summaries, *s)
	}

	return summaries, nil
}
