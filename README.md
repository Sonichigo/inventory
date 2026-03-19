# BBQBookkeeper

> A intentionally load-heavy BBQ inventory API used to demonstrate database performance degradation and monitoring with DBMarlin on Kubernetes.

---

## What is this?

BBQBookkeeper is a demo application purpose-built for [DBMarlin](https://www.dbmarlin.com) showcasing. It simulates a real-world BBQ restaurant chain managing inventory across multiple locations (Seattle, Portland, Austin, Nashville, San Francisco).

The app comes with a **built-in load generator sidecar** that hammers the `/inventory-by-location` endpoint every 50ms, creating a continuous stream of database queries. This generates the kind of real, visible load that DBMarlin is designed to detect, analyse, and help you optimise — making it ideal for live demos, workshops, and performance monitoring walkthroughs.

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

---

## What DBMarlin Will Show

| Metric | Bad state | Good state | 
--- | --- | --- | 
| Query plan |Sequential scan | Index scan | 
| Avg query time |Several ms|Sub-ms| 
| Total time |High|Drops significantly| 
| Top statement |SELECT ... WHERE LOWER(location)|Same query, faster|

- Top statements — the `/inventory-by-location` query dominates
- Wait events — IO waits from sequential scans across all 3 replicas
- Activity Comparison — before/after the ConfigMap swap shows the inflection point clearly

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
