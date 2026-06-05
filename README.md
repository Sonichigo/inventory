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

---

## Harness Pipeline Flow

The demo uses a Harness pipeline to orchestrate deployment and monitoring. Pipeline identifier: `DB_Marlin_Bad_Performance_FullStory`

### Stage 1: Deploy DB and Application - Performance Issue

Deploys the bad performance state to demonstrate issues.

**Deploy DB Step Group:**
- Apply Database Schema (Liquibase): bookkeeper schema on postgres instance, tag v1.0.2
- Notify Changes to DBMarlin: POST event to DBMarlin API with execution URL

**Deploy Application:** K8s rolling deployment of bad version

**Verify the Deployment:** Load test verification (10m, MEDIUM sensitivity, baseline LAST)
- Failure strategy: Manual intervention on verification failure

**Rollback:** Automatic stage rollback on errors

### Stage 2: Approval

Manual gate requiring 1 approver to confirm performance issues before remediation (2-day timeout).

### Stage 3: BBQBook Keeper - New Image

Deploys the optimized version to demonstrate performance improvements.

**Deploy DB Step Group:**
- Apply New DB Schema (Liquibase): bookkeeper schema on goodinstance, tag v1.0.0
- Notify Changes: Template-based DBMarlin notification

**New Application Deployment:** K8s rolling deployment of good version

**New Deployment Verification:** Load test (5m, HIGH sensitivity, baseline LAST)
- Failure strategy: Manual intervention on verification failure

**Rollback:** Automatic stage rollback on errors

---

## Demo Reset Flow

To reset the demo and build a good baseline before showing performance degradation:

### Reset Steps

1. Update the Harness service YAML manifest reference from `k8s-deploy-bad.yml` to `k8s-deploy.yml`
2. Run the pipeline to build a clean baseline in the CV service
3. Revert the service YAML back to `k8s-deploy-bad.yml` before starting the demo

### Why This Matters

- CV service has good performance metrics as a reference point
- Bad deployment can be compared against the good baseline
- Each demo starts from the same known-good state
- DBMarlin shows clear before/after performance degradation

**Note:** Only the service manifest reference needs to change between `k8s-deploy.yml` (good) and `k8s-deploy-bad.yml` (bad). No code changes or image rebuilds required.

---

## End-to-End Demo Setup Guide

This section provides a complete walkthrough from scratch to a working demo with Harness pipeline integration.

### Prerequisites Checklist

- [ ] Kubernetes cluster running and `kubectl` configured
- [ ] Docker installed for building images
- [ ] Harness account with pipeline access
- [ ] DBMarlin instance running and accessible
- [ ] Liquibase configured in Harness (for DB schema management)
- [ ] Harness connectors configured:
  - Docker registry connector
  - Kubernetes cluster connector (GKE or equivalent)
  - Harness image connector (for Liquibase images)

### Step 1: Initial Infrastructure Setup

**1.1 Deploy PostgreSQL Database**
```bash
# Navigate to project directory
cd /path/to/Inventory

# Deploy Postgres with PVC
kubectl apply -f postgres.yaml

# Wait for Postgres to be ready
kubectl rollout status deployment/postgres-dbops -n default
kubectl get pods -n default -l app=postgres-dbops
```

**1.2 Configure DBMarlin to Monitor the Database**
- Point DBMarlin to your PostgreSQL endpoint: `postgres-dbops:5432`
- Configure monitoring credentials (user/password from postgres.yaml)
- Verify DBMarlin can connect and is collecting metrics

### Step 2: Build and Push Application Images

**2.1 Build both versions (bad and good)**
```bash
# Build bad version
docker build --build-arg BUILD_VERSION=bad -t ghcr.io/sonichigo/bbqbookkeeper:bad .
docker push ghcr.io/sonichigo/bbqbookkeeper:bad

# Build good version
docker build --build-arg BUILD_VERSION=good -t ghcr.io/sonichigo/bbqbookkeeper:good .
docker push ghcr.io/sonichigo/bbqbookkeeper:good

# Or use the build script
./build-and-push.sh
```

**2.2 Update image references if using a different registry**
```yaml
# Edit k8s/k8s-deploy-bad.yaml and k8s/k8s-deploy-good.yaml
# Update the image field to your registry path
image: YOUR_REGISTRY/bbqbookkeeper:bad
```

### Step 3: Configure Harness Pipeline

**3.1 Create Harness Services**

Create two services in Harness (Project: DBMarlin):

**Service 1: `deploy_app` (Bad version)**
- Service type: Kubernetes
- Manifest: Reference `k8s-deploy-bad.yml` from your repo
- Artifact: Docker image `ghcr.io/sonichigo/bbqbookkeeper:bad`

**Service 2: `deploy` (Good version)**
- Service type: Kubernetes  
- Manifest: Reference `k8s-deploy.yml` or `k8s-deploy-good.yml` from your repo
- Artifact: Docker image `ghcr.io/sonichigo/bbqbookkeeper:good`

**3.2 Create Harness Environment**

- Environment name: `dsfd` (or your preferred name)
- Environment type: Pre-Production or Production
- Infrastructure definition: `DemoGKE` (or your K8s cluster)
  - Connector: Your GKE/K8s connector
  - Namespace: `default`

**3.3 Configure Liquibase DB Instances**

In Harness DB DevOps, create two database instances:

**Instance 1: `postgres` (Bad schema)**
- Database: `bookkeeper`
- Context: `bad`
- Schema version: `v1.0.2`
- Configuration: No indexes, 50k rows

**Instance 2: `goodinstance` (Good schema)**
- Database: `bookkeeper`
- Context: `good`
- Schema version: `v1.0.0`
- Configuration: With indexes, 10k rows

**3.4 Create DBMarlin Notification Template**

Create template `Notify_DBmarlin`:
```bash
curl -s -X POST \
  -H 'Content-Type: application/json' \
  -d "[{\"startDateTime\":\"$(date -u +%Y-%m-%dT%H:%M:%S.000Z)\",\"databaseTargetId\":2,\"eventTypeId\":5,\"title\":\"Execution of <+pipeline.name>\",\"detailsUrl\":\"<+pipeline.executionUrl>\"}]" \
  http://YOUR_DBMARLIN_IP:9090/archiver/rest/v1/event \
  -v -w 'Response time: %{time_total}s\n'
```

**3.5 Import the Pipeline**

- Copy the pipeline YAML provided in this README (Harness Pipeline Flow section)
- Import into Harness (Project: DBMarlin, Org: default)
- Pipeline identifier: `DB_Marlin_Bad_Performance_FullStory`
- Update the DBMarlin API endpoint in the "Notify Changes" step

### Step 4: Reset Demo - Build Good Baseline

Before running the actual demo, establish a clean baseline:

**4.1 Update Service to Good Version**
```yaml
# In Harness service `deploy_app`, update manifest reference:
# FROM: k8s-deploy-bad.yml
# TO:   k8s-deploy.yml
```

**4.2 Run Baseline Pipeline**
- Execute the Harness pipeline
- This deploys the good version first
- CV service records healthy performance metrics
- DBMarlin captures baseline query patterns

**4.3 Verify Baseline in DBMarlin**
- Check average query time: 5-50ms
- Query plan: Index scans
- Connection pool: Healthy
- Take note of these metrics as your baseline

**4.4 Revert Service Back to Bad Version**
```yaml
# In Harness service `deploy_app`, change manifest back:
# FROM: k8s-deploy.yml  
# TO:   k8s-deploy-bad.yml
```

### Step 5: Run the Demo

**5.1 Execute the Pipeline**
- Go to Harness pipeline: `DB_Marlin_Bad_Performance_FullStory`
- Click "Run Pipeline"
- Stage 1 deploys the bad version with performance issues

**5.2 Observe Stage 1: Performance Issue**
- Monitor the pipeline execution
- Watch DBMarlin metrics:
  - Query time increases to 500-5000ms
  - Sequential scans on 50k rows
  - N+1 query pattern visible
  - Connection pool exhaustion
- CV verification step compares against baseline (MEDIUM sensitivity, 10m duration)
- Explain to audience: "This is what happens with poor database design"

**5.3 Stage 2: Manual Approval**
- Review the performance degradation in DBMarlin
- Show the Activity Comparison view (baseline vs bad)
- Point out specific issues:
  - No indexes on `LOWER()` columns
  - 50k rows loaded per query
  - N+1 query pattern
- Approve the pipeline to proceed to remediation

**5.4 Stage 3: Deploy Fix**
- Pipeline automatically deploys good version
- Watch DBMarlin metrics improve in real-time:
  - Query time drops to 5-50ms
  - Index scans replace sequential scans
  - Single JOIN replaces N+1 queries
  - Connection pool healthy
- CV verification step confirms improvement (HIGH sensitivity, 5m duration)

**5.5 Final Comparison**
- Show DBMarlin Activity Comparison:
  - Before: Sequential scan, 500-5000ms
  - After: Index scan, 5-50ms
  - 100-1000x performance improvement
- Highlight the schema changes in Liquibase (added indexes)
- Show the application code changes (N+1 to JOIN)

### Step 6: Teardown (Optional)

```bash
# Delete application deployments
kubectl delete -f k8s/k8s-deploy-bad.yaml
kubectl delete -f k8s/k8s-deploy-good.yaml

# Delete database (WARNING: loses all data)
kubectl delete -f postgres.yaml
```

---

## Troubleshooting

### Pipeline Fails at DB Schema Apply

**Issue:** Liquibase step fails with "schema not found"

**Solution:**
- Check DB instance configuration in Harness
- Verify database name is `bookkeeper`
- Ensure Postgres is running: `kubectl get pods -l app=postgres-dbops`

### DBMarlin Not Receiving Events

**Issue:** Notification step succeeds but events don't appear in DBMarlin

**Solution:**
- Verify DBMarlin endpoint is accessible from K8s cluster
- Check databaseTargetId matches your DBMarlin configuration
- Confirm eventTypeId=5 exists in your DBMarlin instance

### CV Verification Fails

**Issue:** Verification step fails immediately or times out

**Solution:**
- Ensure baseline exists (run reset flow first)
- Check monitored service is configured correctly
- Verify metrics are flowing from K8s to CV
- Review sensitivity setting (MEDIUM/HIGH)

### Application Pods Not Starting

**Issue:** Pods in CrashLoopBackOff or ImagePullBackOff

**Solution:**
- Check image exists in registry: `docker pull ghcr.io/sonichigo/bbqbookkeeper:bad`
- Verify image pull secrets if using private registry
- Check logs: `kubectl logs -l app=bbqbookkeeper`
- Ensure Postgres is reachable: `kubectl exec -it <pod> -- ping postgres-dbops`

### Load Generator Not Creating Load

**Issue:** No queries visible in DBMarlin

**Solution:**
- Check sidecar container is running: `kubectl get pods -o jsonpath='{.items[*].spec.containers[*].name}'`
- Verify load generator container logs: `kubectl logs <pod> -c load-generator`
- Confirm app is responding: `kubectl exec -it <pod> -- curl localhost:8080/health`

---

## Quick Reference

### Important Files
- `postgres.yaml` - PostgreSQL deployment
- `k8s/k8s-deploy-bad.yaml` - Bad version K8s manifest
- `k8s/k8s-deploy-good.yaml` - Good version K8s manifest  
- `k8s/k8s-deploy.yaml` - Generic manifest (for baseline)
- Liquibase changelogs with `--contexts=bad` and `--contexts=good`

### Key Metrics to Watch in DBMarlin
- Average query time: 500-5000ms (bad) → 5-50ms (good)
- Query plan: Sequential scan (bad) → Index scan (good)
- Queries per minute: 10,000+ (bad) → 100-200 (good)
- Connection pool: Exhausted (bad) → Healthy (good)

### Pipeline Stages at a Glance
1. **Deploy Bad + Verify** → Shows the problem
2. **Manual Approval** → Time to analyze issues
3. **Deploy Good + Verify** → Shows the fix

### Reset Before Each Demo
1. Service YAML: `k8s-deploy-bad.yml` → `k8s-deploy.yml`
2. Run pipeline once
3. Service YAML: `k8s-deploy.yml` → `k8s-deploy-bad.yml`
4. Ready to demo
