# BBQBookkeeper — Go + PostgreSQL

A Go rewrite of the BBQBookkeeper inventory service backed by PostgreSQL instead of MSSQL.

## Project layout

```
bbqbookkeeper/
├── cmd/server/main.go          # entrypoint, reads env vars
├── internal/
│   ├── db/db.go                # postgres init, migrations, all queries
│   ├── handlers/handlers.go    # HTTP handlers
│   └── models/models.go        # shared structs
├── Dockerfile                  # multi-stage, final image ~8 MB
├── k8s-deploy.yaml             # full k8s stack (namespace → postgres → app)
└── go.mod
```

## API endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness + DB ping |
| GET | `/inventory-by-location?location=Seattle` | Items at a location |
| GET | `/inventory` | All items |
| POST | `/inventory` | Add item `{"name":"Brisket","quantity":10,"location":"Seattle","unit":"lbs"}` |
| PUT | `/inventory/{id}` | Update quantity `{"quantity":20}` |
| DELETE | `/inventory/{id}` | Remove item |
| GET | `/locations` | All locations |

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_INIT_USER` | `postgres` | Admin user (creates DB + app role) |
| `DB_INIT_PASS` | `postgres` | Admin password |
| `DB_SERVER` | `localhost` | Postgres host or K8s service name |
| `DB_PORT` | `5432` | Postgres port |
| `DB_USER` | `porxie` | App-level DB user (created if missing) |
| `DB_PASSWORD` | `P0rx!e24` | App-level DB password |
| `DB_NAME` | `bbqbookkeeper` | Database name (created if missing) |

## Build & push

```bash
# 1. Get dependencies
go mod tidy

# 2. Build locally to verify
go build ./cmd/server

# 3. Build & push Docker image
docker build -t YOUR_REGISTRY/bbqbookkeeper:latest .
docker push YOUR_REGISTRY/bbqbookkeeper:latest
```

## Deploy to Kubernetes

```bash
# 1. Update the image name in k8s-deploy.yaml
#    image: YOUR_REGISTRY/bbqbookkeeper:latest

# 2. Apply everything (namespace → secrets → PVC → postgres → app)
kubectl apply -f k8s-deploy.yaml

# 3. Watch rollout
kubectl rollout status deployment/bbqbookeeper-web -n bbq-bookkeeper

# 4. Test the endpoint
kubectl get svc bookeeper-web -n bbq-bookkeeper   # get EXTERNAL-IP
curl http://<EXTERNAL-IP>:8080/inventory-by-location?location=Seattle
```

## Run locally (requires a running Postgres)

```bash
# Start Postgres in Docker
docker run -d --name pg \
  -e POSTGRES_PASSWORD=Porxie24 \
  -p 5432:5432 postgres:15-alpine

# Run the app
DB_INIT_PASS=Porxie24 DB_INIT_USER=postgres \
DB_USER=porxie DB_PASSWORD='P0rx!e24' \
go run ./cmd/server

# Query it
curl "http://localhost:8080/inventory-by-location?location=Seattle"
```
