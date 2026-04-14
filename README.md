# BBQBookkeeper

> A intentionally load-heavy BBQ inventory API demonstrating **BOTH application-level AND database-level** performance issues for DBMarlin monitoring on Kubernetes.

---

## What is this?

BBQBookkeeper is a demo application purpose-built for [DBMarlin](https://www.dbmarlin.com) showcasing. It simulates a real-world BBQ restaurant chain managing inventory across multiple locations (Seattle, Portland, Austin, Nashville, San Francisco).

**NEW:** This demo now includes **TWO versions** of the application:
- 🔴 **BAD** - Application with N+1 queries, missing indexes, poor connection pooling
- 🟢 **GOOD** - Optimized application with JOINs, proper indexes, efficient queries

The app comes with a **built-in load generator sidecar** that hammers the `/inventory-by-location` endpoint continuously, creating a real stream of database queries. This showcases how **both** application code AND database configuration affect performance — making it ideal for live demos, workshops, and performance monitoring walkthroughs.

📖 **See [PERFORMANCE_DEMO.md](PERFORMANCE_DEMO.md) for detailed bad vs good comparison**  
📖 **See [ANTI_PATTERNS.md](ANTI_PATTERNS.md) for specific anti-patterns introduced**

---

## Architecture

```
┌─────────────────────────────────────────┐
│           Kubernetes Pod (x3)           │
│                                         │
│  ┌──────────────────┐  ┌─────────────┐  │
│  │  load-generator  │  │ bbqinventory│  │
│  │  (curl sidecar)  │─▶│ app :8080   │  │
│  │  req every 50ms  │  │  (Go)       │  │
│  └──────────────────┘  └──────┬──────┘  │
└─────────────────────────────── │ ───────┘
                                 │
                    ┌────────────▼────────────┐
                    │   PostgreSQL :5432       │
                    │   (persistent PVC)       │
                    └─────────────────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │        DBMarlin          │
                    │  (monitoring & analysis) │
                    └─────────────────────────┘
```

---

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | App + DB health check |
| `GET` | `/inventory-by-location?location=Seattle` | Get inventory for a location |
| `GET` | `/inventory` | Get all inventory items |
| `POST` | `/inventory` | Add a new inventory item |
| `PUT` | `/inventory/{id}` | Update item quantity |
| `DELETE` | `/inventory/{id}` | Remove an item |
| `GET` | `/locations` | List all locations |

---

## Prerequisites

- Kubernetes cluster (local or cloud)
- `kubectl` configured
- Docker (to build and push the image)
- DBMarlin pointed at your PostgreSQL instance
- Liquibase (for database schema management with context switching)

---

## Quick Start

### 1. Build Both Versions
```bash
# Build and push both bad and good images
./build-and-push.sh

# Or build manually:
docker build --build-arg BUILD_VERSION=bad -t ghcr.io/sonichigo/bbqbookkeeper:bad .
docker build --build-arg BUILD_VERSION=good -t ghcr.io/sonichigo/bbqbookkeeper:good .
```

### 2. Deploy Bad State (Demo Performance Issues)
```bash
# Deploy bad application
kubectl apply -f k8s/k8s-deploy-bad.yaml

# Run Liquibase with bad context (50k rows, no indexes)
# Ensure your Liquibase pipeline uses: --contexts=bad
```

Access bad app: `http://<cluster-ip>:30003`

### 3. Deploy Good State (Show Performance Fix)
```bash
# Deploy good application
kubectl apply -f k8s/k8s-deploy-good.yaml

# Run Liquibase with good context (10k rows, with indexes)
# Ensure your Liquibase pipeline uses: --contexts=good
```

Access good app: `http://<cluster-ip>:30002`

### 4. Monitor Performance Difference
```bash
# Check bad version metrics
curl http://<bad-service>:8080/metrics | jq .avg_response_ms
# Expected: 500-5000ms

# Check good version metrics
curl http://<good-service>:8080/metrics | jq .avg_response_ms
# Expected: 5-50ms
```

---

## Performance Comparison

### 🔴 Bad State (Application + Database)
**Application Issues:**
- N+1 queries (1000s of separate queries)
- No LIMIT clauses (loads all 50k rows)
- In-memory filtering instead of SQL WHERE
- Connection pool: only 2 connections
- Manual aggregation (nested loops)

**Database Issues:**
- 50,000 inventory + 50,000 supplier rows
- NO indexes on LOWER() columns
- Sequential scans on every query
- No deduplication (JOIN fanout)

**Result:** Response times in **SECONDS**, 100% CPU usage

### 🟢 Good State (Application + Database)
**Application Fixes:**
- Efficient JOINs (1 query replaces 1000s)
- LIMIT clauses on large queries
- SQL WHERE filtering
- Connection pool: 25 connections
- SQL GROUP BY aggregations

**Database Fixes:**
- 10,000 inventory + 10,000 supplier rows
- Functional indexes on all LOWER() columns
- Index scans on every query
- Deduplicated suppliers

**Result:** Response times in **MILLISECONDS**, <10% CPU usage

### What DBMarlin Will Show

| Metric | Bad State | Good State | 
|--------|-----------|------------|
| Query plan | Sequential scan (50k rows) | Index scan (<100 rows) |
| Avg query time | 500-5000ms | 5-50ms |
| Query count | 10,000+ queries/min (N+1) | 100-200 queries/min (JOINs) |
| Top statement | SELECT ... (sequential scan) | SELECT ... (index scan) |
| Wait events | IO waits, lock waits | Minimal waits |
| Connection pool | Exhausted (queued) | Healthy (active) |

**Speedup:** 100-1000x faster! ⚡

## Project Structure

```
inventory/
├── main.go             # Entrypoint — reads SQL_DIR and DB_SERVER env vars
├── db.go               # Postgres bootstrap + runSQLFile() loader
├── handlers.go         # HTTP route handlers
├── models.go           # Shared structs
├── go.mod / go.sum     # Go module
├── Dockerfile          # Multi-stage build, embeds ui/ (~10MB final image)
├── postgres.yaml       # Postgres PVC, Deployment, Service (deploy first)
├── k8s-deploy.yaml     # App Deployment + Service (mounts ConfigMap as SQL_DIR)
├── ui/
│   └── index.html      # Demo query driver UI — served at /ui/
├── sql/
│   ├── schema.sql      # Table definitions — runs once on startup
│   ├── seed-bad.sql    # No index → sequential scan (the problem)
│   └── seed-good.sql   # Functional index added (the fix)
└── k8s/
    ├── configmap-bad.yaml   # Mounts schema.sql + seed-bad.sql
    └── configmap-good.yaml  # Mounts schema.sql + seed-good.sql
```

## How the SQL ConfigMap works
The app reads two SQL files at startup from the directory set by SQL_DIR (default: /etc/bbq-sql):

- `schema.sql` — creates tables and seeds location data (idempotent, safe to re-run)
- `seed-bad.sql` — the problematic seed data (no index)
- `seed-good.sql` — the fixed seed data (with functional index)

Swapping the ConfigMap and restarting the deployment is all it takes to flip between the degraded and fixed states — no Docker rebuild needed.

---

## Files

```
inventory/
├── postgres.yaml   # Postgres PVC, ConfigMap, Deployment, Service
└── k8s-deploy.yaml # BBQBookkeeper app Deployment + Service
```

> **Important:** `postgres.yaml` and `k8s-deploy.yaml` are intentionally separate.
> Always deploy Postgres first and confirm it is ready before deploying the app.

---

## Run Locally (Dev)

**1. Start Postgres in Docker:**
```bash
docker run -d --name pg \
  -e POSTGRES_USER=user \
  -e POSTGRES_PASSWORD=password \
  -e POSTGRES_DB=mydatabase \
  -p 5432:5432 postgres:14
```

**2. Run the app:**
```bash
go mod tidy
go mod tidy
SQL_DIR=./sql go run .
```

**3. Test it:**
```bash
curl "http://localhost:8080/inventory-by-location?location=Seattle"
curl "http://localhost:8080/health"
```

---

## Deploy to Kubernetes (Prod)

### Deploy Order — Postgres first, then the app

**Step 1 — Deploy Postgres:**
```bash
cd inventory && kubectl apply -f postgres.yaml
```

**Step 2 — Wait for Postgres to be ready:**
```bash
kubectl rollout status deployment/postgres-dbops -n default
kubectl get pods -n default -l app=postgres-dbops
```

> Postgres exposes itself inside the cluster as `postgres-dbops:5432`.
> The app is pre-configured to connect to this service name via `DB_SERVER=postgres-dbops`.

**Step 3 — Build and push the app image:**
```bash
docker build -t YOUR_REGISTRY/bbqbookkeeper:latest .
docker push YOUR_REGISTRY/bbqbookkeeper:latest
```

**Step 4 — Update the image in the manifest:**
```yaml
# k8s-deploy.yaml
image: YOUR_REGISTRY/bbqbookkeeper:latest
```

**Step 5 — Apply the bad ConfigMap (starting state) and Deploy the app:**
```bash
kubectl apply -f k8s/configmap-bad.yaml
kubectl apply -f k8s-deploy.yaml
kubectl rollout status deployment/bbqbookeeper-web -n default
```

- Open the UI and enable Auto Blast
- Switch to DBMarlin — watch executions climb, average time increase
- Show the `seed-bad.sql` file — point out no index, `LOWER()` wrapping

**Step 6 — Get the external IP and test:**
```bash
kubectl get svc bbqbookkeeper-web -n default
# Open http://<EXTERNAL-IP>:8080/ui/ in your browser to access the demo UI
```

**Step 7 — Swap to the good ConfigMap to fix the issue:**
```bash
kubectl apply -f k8s/configmap-good.yaml
kubectl rollout restart deployment/bbqbookeeper-web -n default
```

- Stay on DBMarlin — watch average time drop as pods roll over
- Show the `seed-good.sql` file — point out `CREATE INDEX ... ON inventory (LOWER(location))`
- Use DBMarlin's Activity Comparison view to show before vs after side by side
