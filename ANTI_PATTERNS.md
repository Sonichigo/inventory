# Application Anti-Patterns: Bad vs Good

## Overview
This document details the specific performance anti-patterns introduced in the "bad" version of the application.

---

## 1. N+1 Query Problem

### Bad Version ([db_bad.go:112-154](db_bad.go#L112-L154))
```go
func (d *DB) GetInventoryByLocation(location string) ([]InventoryItem, error) {
    // Query 1: Fetch inventory items
    rows := d.conn.Query("SELECT id, name, quantity, location, unit FROM inventory WHERE ...")
    
    for rows.Next() {
        // Query 2, 3, 4, ... N: One query PER inventory item!
        d.conn.QueryRow(
            "SELECT name, lead_days FROM suppliers WHERE location = $1 AND item = $2",
            item.Location, item.Name,
        ).Scan(&supplierName, &leadDays)
    }
}
```

**Impact:** With 1000 inventory items → **1001 queries**  
**Cost:** Each query ~5-10ms = **5-10 seconds** total

### Good Version ([db.go:112-143](db.go#L112-L143))
```go
func (d *DB) GetInventoryByLocation(location string) ([]InventoryItem, error) {
    // Single query with JOIN
    rows := d.conn.Query(`
        SELECT i.id, i.name, i.quantity, i.location, i.unit,
               COALESCE(s.name, ''), COALESCE(s.lead_days, 0)
        FROM inventory i
        LEFT JOIN suppliers s ON LOWER(s.location) = LOWER(i.location)
                             AND LOWER(s.item) = LOWER(i.name)
        WHERE LOWER(i.location) = LOWER($1)
    `, location)
}
```

**Impact:** 1000 items → **1 query**  
**Cost:** Single query ~20-50ms

**Speedup:** 100-200x faster! ⚡

---

## 2. Missing LIMIT Clause

### Bad Version ([db_bad.go:157-170](db_bad.go#L157-L170))
```go
func (d *DB) GetAllInventory() ([]InventoryItem, error) {
    // Loads ALL 50,000+ rows into memory
    rows := d.conn.Query(
        "SELECT id, name, quantity, location, unit FROM inventory ORDER BY location, name",
    )
}
```

**Impact:**
- Transfers 50,000 rows over network
- Allocates ~50MB+ in Go memory
- Query takes 500-1000ms

### Good Version ([db.go:145-163](db.go#L145-L163))
```go
func (d *DB) GetAllInventory() ([]InventoryItem, error) {
    // Limits to 100 rows for display
    rows := d.conn.Query(
        "SELECT id, name, quantity, location, unit FROM inventory ORDER BY location, name LIMIT 100",
    )
}
```

**Impact:**
- Transfers only 100 rows
- Allocates ~100KB in memory
- Query takes 5-10ms

**Speedup:** 50-100x faster! ⚡

---

## 3. In-Memory Filtering (Bad SQL Pushdown)

### Bad Version ([db_bad.go:222-266](db_bad.go#L222-L266))
```go
func (d *DB) GetLowStock(threshold int) ([]InventoryItem, error) {
    // Fetches ALL inventory (no WHERE clause)
    rows := d.conn.Query("SELECT * FROM inventory ORDER BY quantity")
    
    for rows.Next() {
        // Filter in Go instead of SQL
        if item.Quantity >= threshold {
            continue  // Skip in Go
        }
        
        // ALSO does N+1 query for supplier!
        d.conn.QueryRow("SELECT name, lead_days FROM suppliers ...")
    }
}
```

**Impact:**
- Loads all 50,000 rows
- Filters 99% of data in Go (wasted work)
- 50,000 additional supplier queries (N+1)
- Total time: **10-30 seconds**

### Good Version ([db.go:222-253](db.go#L222-L253))
```go
func (d *DB) GetLowStock(threshold int) ([]InventoryItem, error) {
    // Filters in SQL with WHERE clause
    rows := d.conn.Query(`
        SELECT i.*, COALESCE(s.name, ''), COALESCE(s.lead_days, 0)
        FROM inventory i
        LEFT JOIN suppliers s ON ...
        WHERE i.quantity < $1  -- Filter in SQL!
        ORDER BY i.quantity ASC
    `, threshold)
}
```

**Impact:**
- Database filters rows (typically 100-200 matches)
- Single JOIN query
- Total time: **10-50ms**

**Speedup:** 200-3000x faster! ⚡

---

## 4. Reduced Connection Pool

### Bad Version ([db_bad.go:86-88](db_bad.go#L86-L88))
```go
// Only 2 concurrent connections allowed
appConn.SetMaxOpenConns(2)
appConn.SetMaxIdleConns(1)
appConn.SetConnMaxLifetime(1 * time.Minute)
```

**Impact:**
- With 10 concurrent requests → 8 queued, waiting for connections
- Load generator creates artificial bottleneck
- Timeouts and 503 errors under load

### Good Version ([db.go:82-84](db.go#L82-L84))
```go
// 25 concurrent connections
appConn.SetMaxOpenConns(25)
appConn.SetMaxIdleConns(5)
appConn.SetConnMaxLifetime(5 * time.Minute)
```

**Impact:**
- Handles 25 concurrent requests
- No queuing under normal load
- Stable response times

**Speedup:** 10x higher throughput 🚀

---

## 5. Manual Aggregation (Bad SQL Pushdown)

### Bad Version ([db_bad.go:269-348](db_bad.go#L269-L348))
```go
func (d *DB) GetSupplierSummary(location string) ([]SupplierSummary, error) {
    // Query 1: Fetch ALL suppliers (no WHERE)
    suppRows := d.conn.Query("SELECT name, location, item, lead_days FROM suppliers")
    
    // Filter in Go
    for suppRows.Next() {
        if strings.EqualFold(s.location, location) {
            allSuppliers = append(allSuppliers, s)
        }
    }
    
    // Query 2: Fetch ALL inventory (no JOIN, no WHERE)
    invRows := d.conn.Query("SELECT name, location, quantity FROM inventory")
    
    // Manual aggregation with nested loops (O(N*M) complexity!)
    for _, s := range allSuppliers {
        for _, inv := range allInventory {
            if strings.EqualFold(inv.location, s.location) && 
               strings.EqualFold(inv.name, s.item) {
                summary.TotalStock += inv.quantity
            }
        }
    }
}
```

**Impact:**
- Loads 50k suppliers + 50k inventory = 100k rows
- Nested loop: 50k × 50k = **2.5 BILLION comparisons** 💀
- All in Go memory (100MB+)
- Takes **minutes** to complete

### Good Version ([db.go:257-290](db.go#L257-L290))
```go
func (d *DB) GetSupplierSummary(location string) ([]SupplierSummary, error) {
    // Single query with JOIN + GROUP BY
    rows := d.conn.Query(`
        SELECT s.name, s.location,
               COUNT(DISTINCT i.name) AS item_count,
               AVG(s.lead_days) AS avg_lead_days,
               COALESCE(SUM(i.quantity), 0) AS total_stock
        FROM suppliers s
        LEFT JOIN inventory i ON LOWER(i.location) = LOWER(s.location)
                             AND LOWER(i.name) = LOWER(s.item)
        WHERE LOWER(s.location) = LOWER($1)
        GROUP BY s.name, s.location
        ORDER BY total_stock DESC
    `, location)
}
```

**Impact:**
- Database does aggregation (optimized C code)
- Uses indexes for JOIN
- Returns only aggregated results (~10-20 rows)
- Takes **10-50ms**

**Speedup:** 10,000x+ faster! 🚀🚀🚀

---

## Summary Table

| Anti-Pattern | Bad Performance | Good Performance | Speedup |
|--------------|----------------|------------------|---------|
| N+1 Queries | 5-10s | 20-50ms | **100-200x** |
| No LIMIT | 500-1000ms | 5-10ms | **50-100x** |
| In-Memory Filter | 10-30s | 10-50ms | **200-3000x** |
| Small Conn Pool | Queued/timeout | Concurrent | **10x throughput** |
| Manual Aggregation | Minutes | 10-50ms | **10,000x+** |

---

## Combined Effect

When ALL anti-patterns are present (as in bad state):
- **Single request:** 10-30 seconds
- **Concurrent requests:** Timeouts, connection exhaustion
- **Database load:** 100% CPU, millions of sequential scans
- **Application load:** 100% CPU, OOM risk

When ALL optimizations are present (as in good state):
- **Single request:** 10-50 milliseconds
- **Concurrent requests:** Stable, predictable
- **Database load:** <10% CPU, all index scans
- **Application load:** <5% CPU, low memory

**Overall speedup:** 100-1000x faster end-to-end! 🎯

---

## How to Verify

### Compare Query Plans
```bash
# Bad database (no indexes, sequential scans)
EXPLAIN ANALYZE SELECT ... FROM inventory i LEFT JOIN suppliers s ...
# -> Seq Scan on inventory (cost=0.00..5000.00)
# -> Seq Scan on suppliers (cost=0.00..5000.00)

# Good database (with indexes)
EXPLAIN ANALYZE SELECT ... FROM inventory i LEFT JOIN suppliers s ...
# -> Index Scan using idx_inventory_location_lower (cost=0.42..8.44)
# -> Index Scan using idx_suppliers_location_lower (cost=0.42..8.44)
```

### Monitor Metrics
```bash
# Watch /metrics endpoint
watch -n 1 'curl -s http://localhost:8080/metrics | jq .avg_response_ms'

# Bad version: 500-5000ms
# Good version: 5-50ms
```

### Count Queries
```bash
# Bad version: 10,000+ queries/min (N+1 problem)
# Good version: 100-200 queries/min (JOINs)
```
