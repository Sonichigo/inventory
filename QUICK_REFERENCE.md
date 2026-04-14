# Quick Reference: Bad vs Good Demo

## TL;DR

**Bad State:** 🔴 Application N+1 queries + Database no indexes = **SECONDS**  
**Good State:** 🟢 Application JOINs + Database with indexes = **MILLISECONDS**

---

## Build Commands

```bash
# Build both versions
./build-and-push.sh

# Or manually:
docker build --build-arg BUILD_VERSION=bad -t ghcr.io/sonichigo/bbqbookkeeper:bad .
docker build --build-arg BUILD_VERSION=good -t ghcr.io/sonichigo/bbqbookkeeper:good .
docker push ghcr.io/sonichigo/bbqbookkeeper:bad
docker push ghcr.io/sonichigo/bbqbookkeeper:good
```

---

## Deploy Commands

```bash
# Deploy bad state
kubectl apply -f k8s/k8s-deploy-bad.yaml
# Ensure Liquibase runs with: --contexts=bad

# Deploy good state
kubectl apply -f k8s/k8s-deploy-good.yaml
# Ensure Liquibase runs with: --contexts=good

# Deploy both for comparison
kubectl apply -f k8s/k8s-deploy-bad.yaml -f k8s/k8s-deploy-good.yaml
```

---

## Access Points

| Version | Service | NodePort | URL |
|---------|---------|----------|-----|
| Bad | `bbqbookkeeper-web-bad` | 30003 | `http://<cluster-ip>:30003` |
| Good | `bbqbookkeeper-web-good` | 30002 | `http://<cluster-ip>:30002` |

---

## Key Endpoints

```bash
# Health check
curl http://localhost:8080/health

# Metrics (shows avg response time)
curl http://localhost:8080/metrics | jq .avg_response_ms

# Hot query (most affected by bad state)
curl "http://localhost:8080/inventory-by-location?location=Seattle"

# Aggregation query
curl "http://localhost:8080/supplier-summary?location=Seattle"

# Low stock (full table scan in bad state)
curl "http://localhost:8080/inventory/low-stock?threshold=20"
```

---

## Expected Metrics

| Metric | Bad State | Good State |
|--------|-----------|------------|
| **Response Time** | 500-5000ms | 5-50ms |
| **Query Count** | 10,000+ queries/min | 100-200 queries/min |
| **DB Connections** | Exhausted (2 max) | Healthy (25 max) |
| **CPU Usage** | 80-100% | 5-15% |
| **Query Plan** | Sequential scan | Index scan |

---

## Demo Script

### 1. Show Bad State (30 sec)
```bash
kubectl apply -f k8s/k8s-deploy-bad.yaml
# Wait for deployment

curl http://<bad-service>:8080/metrics | jq
# Point out: avg_response_ms: 2000-5000ms

# Show DBMarlin:
# - Sequential scans
# - 10,000+ queries per minute
# - Connection pool exhausted
```

### 2. Explain Problems (2 min)
- **Application:** N+1 queries, no LIMIT, in-memory filtering
- **Database:** No indexes, 50k rows, sequential scans
- **Result:** Every query scans 50k rows, makes 1000s of queries

### 3. Show Good State (30 sec)
```bash
kubectl apply -f k8s/k8s-deploy-good.yaml
# Wait for deployment

curl http://<good-service>:8080/metrics | jq
# Point out: avg_response_ms: 10-50ms

# Show DBMarlin:
# - Index scans
# - 100-200 queries per minute
# - Connection pool healthy
```

### 4. Highlight Improvements (1 min)
- **Application:** 1 JOIN query replaces 1000s of N+1 queries
- **Database:** Index scan reads <100 rows instead of 50k
- **Result:** 100-1000x speedup!

**Total demo time: ~4 minutes**

---

## Troubleshooting

### Build Issues
```bash
# Check go.mod
go mod tidy

# Test build tags
go build -tags=bad -o test-bad . && ./test-bad
go build -o test-good . && ./test-good
```

### Deployment Issues
```bash
# Check pod logs
kubectl logs -l version=bad -c bbqinventoryapp
kubectl logs -l version=good -c bbqinventoryapp

# Check if images exist
docker images | grep bbqbookkeeper

# Force pull latest
kubectl rollout restart deployment/bbqbookeeper-web-bad
kubectl rollout restart deployment/bbqbookeeper-web-good
```

### Database Connection Issues
```bash
# Check database pods
kubectl get pods -l app=postgres-dbops

# Test connection from pod
kubectl exec -it <bad-pod> -- wget -qO- http://localhost:8080/health
```

### Performance Not Showing Difference
```bash
# Ensure Liquibase contexts match:
# Bad app → Bad DB (contexts=bad)
# Good app → Good DB (contexts=good)

# Check database state
kubectl exec -it <postgres-pod> -- psql -U user -d mydatabase -c "\d+ inventory"
kubectl exec -it <postgres-pod> -- psql -U user -d mydatabase -c "\di"
```

---

## File Reference

| File | Purpose |
|------|---------|
| [db_bad.go](db_bad.go) | Bad version with anti-patterns |
| [db.go](db.go) | Good version (optimized) |
| [Dockerfile](Dockerfile) | Multi-version build support |
| [build-and-push.sh](build-and-push.sh) | Build automation |
| [k8s/k8s-deploy-bad.yaml](k8s/k8s-deploy-bad.yaml) | Bad state deployment |
| [k8s/k8s-deploy-good.yaml](k8s/k8s-deploy-good.yaml) | Good state deployment |
| [PERFORMANCE_DEMO.md](PERFORMANCE_DEMO.md) | Detailed demo guide |
| [ANTI_PATTERNS.md](ANTI_PATTERNS.md) | Anti-pattern details |
| [IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md) | Implementation overview |

---

## Anti-Patterns Quick Summary

1. **N+1 Queries** - 1 query becomes 1000s
2. **No LIMIT** - Loads all 50k rows
3. **In-Memory Filter** - Filters in Go instead of SQL
4. **Small Connection Pool** - Only 2 connections
5. **Manual Aggregation** - Nested loops (O(N×M))

**Fix ALL → 100-1000x speedup!** 🚀

---

## Remember

✅ Always match application version to database context  
✅ Both versions can run simultaneously for comparison  
✅ Load generator runs automatically in sidecar  
✅ Check `/metrics` endpoint for real-time performance data  
✅ DBMarlin shows query-level details (plans, waits, connections)
