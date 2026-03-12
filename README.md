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
go run .
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
kubectl apply -f postgres.yaml
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

**Step 5 — Deploy the app:**
```bash
kubectl apply -f k8s-deploy.yaml
```

**Step 6 — Watch it come up:**
```bash
kubectl rollout status deployment/bbqbookeeper-web -n default
kubectl get pods -n default
```

**Step 7 — Get the external IP and test:**
```bash
kubectl get svc bbqbookkeeper-web -n default
curl "http://<EXTERNAL-IP>:8080/inventory-by-location?location=Austin"
curl "http://<EXTERNAL-IP>:8080/health"
```

---

## What DBMarlin Will Show

Once deployed, DBMarlin will immediately start picking up load from the sidecar's continuous requests. You can use this to demonstrate:

- **Query frequency** — the `/inventory-by-location` query fires ~20 times/second per pod (60/sec across 3 replicas)
- **Slow query detection** — add more data or remove indexes to simulate degradation
- **Wait events** — connection pool pressure from 3 replicas hitting one Postgres pod
- **Before/after comparison** — DBMarlin's time-comparison view shows the impact of adding an index or tuning a query

---

## Project Structure

```
inventory/
├── main.go         # Entrypoint, hardcoded config
├── db.go           # Postgres init, migrations, all queries
├── handlers.go     # HTTP route handlers
├── models.go       # Shared structs
├── go.mod          # Go module
├── Dockerfile      # Multi-stage build (~8MB final image)
├── postgres.yaml   # Postgres PVC, ConfigMap, Deployment, Service
└── k8s-deploy.yaml # BBQBookkeeper app Deployment + Service
```