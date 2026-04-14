# Performance Demo: Bad vs Good Application + Database

This demo showcases performance degradation caused by **BOTH** application-level and database-level anti-patterns.

## Demo Scenario

### 🔴 BAD State (Performance Disaster)
**Application Anti-Patterns:**
- N+1 query problem (thousands of separate queries instead of JOINs)
- No LIMIT clauses (loading ALL 50k+ rows into memory)
- In-memory filtering instead of SQL WHERE clauses
- Reduced connection pool (2 connections vs 25)
- Manual aggregation in Go instead of SQL GROUP BY
- Nested loops creating O(N*M) complexity

**Database Anti-Patterns:**
- 50,000 inventory rows + 50,000 supplier rows
- NO indexes on LOWER() join columns
- Sequential scans on 100k+ rows
- No supplier deduplication (JOIN fanout)

**Expected Result:** Query response times in **seconds**, high CPU, memory exhaustion

---

### 🟢 GOOD State (Optimized Performance)
**Application Optimizations:**
- Efficient JOINs (1 query instead of 1000s)
- LIMIT clauses on large queries
- SQL-side filtering (WHERE clauses)
- Proper connection pooling (25 connections)
- SQL aggregations (GROUP BY, COUNT, AVG)
- Efficient query patterns

**Database Optimizations:**
- 10,000 inventory rows + 10,000 supplier rows (reasonable scale)
- Functional indexes on all LOWER() columns
- Index scans (sub-millisecond lookups)
- Deduplicated suppliers (no JOIN fanout)

**Expected Result:** Query response times in **milliseconds**, low resource usage

---

## Building the Images

### Option 1: Build Script (Recommended)
```bash
./build-and-push.sh
```

This builds and pushes:
- `ghcr.io/sonichigo/bbqbookkeeper:bad`
- `ghcr.io/sonichigo/bbqbookkeeper:good`
- `ghcr.io/sonichigo/bbqbookkeeper:latest` (alias for good)

### Option 2: Manual Docker Build
```bash
# Bad version
docker build --build-arg BUILD_VERSION=bad -t ghcr.io/sonichigo/bbqbookkeeper:bad .

# Good version
docker build --build-arg BUILD_VERSION=good -t ghcr.io/sonichigo/bbqbookkeeper:good .

# Push
docker push ghcr.io/sonichigo/bbqbookkeeper:bad
docker push ghcr.io/sonichigo/bbqbookkeeper:good
```

---

## Deploying to Kubernetes

### Deploy BAD State
```bash
# Deploy BAD application
kubectl apply -f k8s/k8s-deploy-bad.yaml

# Ensure Liquibase runs with BAD database context
# Your Liquibase pipeline should use: --contexts=bad
```

**Note:** The bad deployment expects database hostname `postgres-dbops-bad`

### Deploy GOOD State
```bash
# Deploy GOOD application
kubectl apply -f k8s/k8s-deploy-good.yaml

# Ensure Liquibase runs with GOOD database context
# Your Liquibase pipeline should use: --contexts=good
```

**Note:** The good deployment expects database hostname `postgres-dbops-good`

### Deploy Both for A/B Comparison
```bash
kubectl apply -f k8s/k8s-deploy-bad.yaml
kubectl apply -f k8s/k8s-deploy-good.yaml
```

This runs both versions side-by-side:
- Bad: `http://<cluster-ip>:30003`
- Good: `http://<cluster-ip>:30002`

---

## Verifying Performance Difference

### Check Metrics Endpoint
```bash
# Bad version
curl http://<bad-service-ip>:8080/metrics | jq

# Good version
curl http://<good-service-ip>:8080/metrics | jq
```

Look for `avg_response_ms`:
- **Bad**: Likely 500-5000ms
- **Good**: Likely 5-50ms

### Monitor Queries
The load generator sidecar continuously hits these endpoints:
- `/inventory-by-location?location={city}` ← Most affected by bad state
- `/supplier-summary?location={city}` ← Heavy aggregation query
- `/inventory/low-stock?threshold=20` ← Full table scan

---

## Application Code Differences

### Bad Version ([db_bad.go](db_bad.go))
**GetInventoryByLocation (N+1 Problem):**
```go
// Step 1: Fetch inventory (no JOIN)
rows := db.Query("SELECT * FROM inventory WHERE location = $1")

// Step 2: For EACH row, query suppliers (50k+ queries!)
for rows.Next() {
    db.QueryRow("SELECT name, lead_days FROM suppliers WHERE location = $1 AND item = $2", ...)
}
```

**GetLowStock (In-Memory Filter):**
```go
// Fetch ALL inventory (50k+ rows)
rows := db.Query("SELECT * FROM inventory")

// Filter in Go instead of SQL
for rows.Next() {
    if item.Quantity >= threshold {
        continue  // Skip in Go
    }
}
```

### Good Version ([db.go](db.go))
**GetInventoryByLocation (Efficient JOIN):**
```go
// Single query with JOIN
db.Query(`
    SELECT i.*, s.name, s.lead_days
    FROM inventory i
    LEFT JOIN suppliers s ON LOWER(s.location) = LOWER(i.location)
                         AND LOWER(s.item) = LOWER(i.name)
    WHERE LOWER(i.location) = LOWER($1)
`)
```

**GetLowStock (SQL Filter):**
```go
// Filter in SQL with WHERE clause
db.Query(`
    SELECT i.*, s.name, s.lead_days
    FROM inventory i
    LEFT JOIN suppliers s ON ...
    WHERE i.quantity < $1
`)
```

---

## Demo Flow

1. **Show Bad State:**
   ```bash
   kubectl apply -f k8s/k8s-deploy-bad.yaml
   # Point DBMarlin/Harness to bad service
   # Watch metrics spike to seconds
   ```

2. **Explain Issues:**
   - Show N+1 queries in DBMarlin query log
   - Show sequential scans in EXPLAIN ANALYZE
   - Show connection pool exhaustion

3. **Deploy Good State:**
   ```bash
   kubectl apply -f k8s/k8s-deploy-good.yaml
   # Switch Harness to good service
   # Watch metrics drop to milliseconds
   ```

4. **Show Improvements:**
   - Show single JOIN queries
   - Show index scans in EXPLAIN ANALYZE
   - Show stable connection pool

---

## Database Context Alignment

| Application Version | Database Context | DB Hostname |
|---------------------|------------------|-------------|
| **Bad**             | `--contexts=bad` | `postgres-dbops-bad` |
| **Good**            | `--contexts=good` | `postgres-dbops-good` |

Ensure your Liquibase pipeline matches application version to database context!

---

## Cleanup

```bash
# Remove bad deployment
kubectl delete -f k8s/k8s-deploy-bad.yaml

# Remove good deployment
kubectl delete -f k8s/k8s-deploy-good.yaml

# Remove both
kubectl delete -f k8s/k8s-deploy-bad.yaml -f k8s/k8s-deploy-good.yaml
```

---

## Key Metrics to Monitor

- **avg_response_ms** (from `/metrics` endpoint)
  - Bad: 500-5000ms
  - Good: 5-50ms

- **DB Connections** (from DBMarlin)
  - Bad: Pool exhaustion, queued connections
  - Good: Stable, low utilization

- **Query Count** (from DBMarlin)
  - Bad: 10,000+ queries/min (N+1 problem)
  - Good: 100-200 queries/min (JOINs)

- **Sequential Scans** (from DBMarlin)
  - Bad: Every query (no indexes)
  - Good: Zero (index scans only)
