package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Handler struct {
	db *DB
}

func NewHandler(database *DB) *Handler {
	return &Handler{db: database}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/inventory-by-location", h.InventoryByLocation)
	mux.HandleFunc("/inventory", h.Inventory)
	mux.HandleFunc("/inventory/", h.InventoryByID)
	mux.HandleFunc("/inventory/low-stock", h.LowStock)
	mux.HandleFunc("/locations", h.Locations)
	mux.HandleFunc("/supplier-summary", h.SupplierSummary)
	// Option A CV endpoint — polled by Harness Custom Health Source
	mux.HandleFunc("/metrics", h.Metrics)
	// DBMarlin proxy — translates Harness epoch ms → DBMarlin date format
	mux.HandleFunc("/dbmarlin-metrics", h.DBMarlinMetrics)
	// DBMarlin proxy — converts Harness epoch ms to DBMarlin date format
	mux.HandleFunc("/dbmarlin/activity", h.DBMarlinActivity)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON encode error: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// GET /health
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	if err := h.db.Ping(); err != nil {
		dbStatus = "unavailable: " + err.Error()
		writeJSON(w, http.StatusServiceUnavailable, HealthResponse{
			Status: "degraded", Database: dbStatus,
		})
		return
	}
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok", Database: dbStatus})
}

// GET /inventory-by-location?location=Seattle
// HOT query — JOIN inventory + suppliers on LOWER() cols, no indexes in bad state
func (h *Handler) InventoryByLocation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	location := strings.TrimSpace(r.URL.Query().Get("location"))
	if location == "" {
		writeError(w, http.StatusBadRequest, "location query parameter is required")
		return
	}
	start := time.Now()
	items, err := h.db.GetInventoryByLocation(location)
	elapsed := time.Since(start).Milliseconds()
	metrics.Record(elapsed, err != nil)

	if err != nil {
		log.Printf("GetInventoryByLocation error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch inventory")
		return
	}
	if items == nil {
		items = []InventoryItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

// GET /inventory        → all items
// POST /inventory       → add item
func (h *Handler) Inventory(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.db.GetAllInventory()
		if err != nil {
			log.Printf("GetAllInventory error: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to fetch inventory")
			return
		}
		if items == nil {
			items = []InventoryItem{}
		}
		writeJSON(w, http.StatusOK, items)

	case http.MethodPost:
		var req AddItemRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Name == "" || req.Location == "" {
			writeError(w, http.StatusBadRequest, "name and location are required")
			return
		}
		if req.Unit == "" {
			req.Unit = "lbs"
		}
		item, err := h.db.AddItem(req)
		if err != nil {
			log.Printf("AddItem error: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to add item")
			return
		}
		writeJSON(w, http.StatusCreated, item)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// PUT /inventory/{id}   → update quantity
// DELETE /inventory/{id} → remove item
func (h *Handler) InventoryByID(w http.ResponseWriter, r *http.Request) {
	// skip if routed to /inventory/low-stock
	if strings.HasSuffix(r.URL.Path, "low-stock") {
		h.LowStock(w, r)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/inventory/"), "/")
	id, err := strconv.Atoi(parts[0])
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid inventory id in path")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var req UpdateQuantityRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		req.ID = id
		item, err := h.db.UpdateQuantity(req)
		if err != nil {
			log.Printf("UpdateQuantity error: %v", err)
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "failed to update item")
			}
			return
		}
		writeJSON(w, http.StatusOK, item)

	case http.MethodDelete:
		if err := h.db.DeleteItem(id); err != nil {
			log.Printf("DeleteItem error: %v", err)
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "failed to delete item")
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// GET /inventory/low-stock?threshold=20
// Aggregation query — GROUP BY location, filtered by quantity threshold
func (h *Handler) LowStock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	threshold := 20
	if t := r.URL.Query().Get("threshold"); t != "" {
		if v, err := strconv.Atoi(t); err == nil {
			threshold = v
		}
	}
	items, err := h.db.GetLowStock(threshold)
	if err != nil {
		log.Printf("GetLowStock error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch low stock")
		return
	}
	if items == nil {
		items = []InventoryItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

// GET /locations
func (h *Handler) Locations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	locs, err := h.db.GetLocations()
	if err != nil {
		log.Printf("GetLocations error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch locations")
		return
	}
	if locs == nil {
		locs = []Location{}
	}
	writeJSON(w, http.StatusOK, locs)
}

// GET /supplier-summary?location=Seattle
// Aggregation — COUNT, AVG lead_days grouped by supplier, joined against inventory
func (h *Handler) SupplierSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	location := strings.TrimSpace(r.URL.Query().Get("location"))
	if location == "" {
		writeError(w, http.StatusBadRequest, "location query parameter is required")
		return
	}
	summary, err := h.db.GetSupplierSummary(location)
	if err != nil {
		log.Printf("GetSupplierSummary error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch supplier summary")
		return
	}
	if summary == nil {
		summary = []SupplierSummary{}
	}
	writeJSON(w, http.StatusOK, summary)
}

// GET /metrics?from=start_time&to=end_time
// Option A CV endpoint — polled by Harness Custom Health Source.
// from/to params accepted but ignored — metrics are rolling window not time-ranged.
// Wrapped in array — Harness CV JSON path requires at least 2 wildcards (*).
// Harness CV thresholds: avg_response_ms > 50 = degraded, > 100 = fail
func (h *Handler) Metrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	// Wrap in array — Harness CV JSON path requires $.*.field notation
	writeJSON(w, http.StatusOK, []MetricsResponse{metrics.Snapshot()})
}

// GET /dbmarlin/activity?from=<epoch_ms>&to=<epoch_ms>
// Proxy endpoint — Harness CV passes epoch ms, DBMarlin expects date strings.
// This converts and forwards to DBMarlin, returning the activity summary.
// Harness CV JSON paths: $.[*].avgwaittime and $.[*].executions
func (h *Handler) DBMarlinActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse from/to as epoch milliseconds from Harness
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	var fromMs, toMs int64
	if fromStr == "" || toStr == "" {
		// fallback: last 10 minutes
		toMs = time.Now().UnixMilli()
		fromMs = toMs - 10*60*1000
	} else {
		var err error
		fromMs, err = strconv.ParseInt(fromStr, 10, 64)
		if err != nil {
			fromMs = time.Now().UnixMilli() - 10*60*1000
		}
		toMs, err = strconv.ParseInt(toStr, 10, 64)
		if err != nil {
			toMs = time.Now().UnixMilli()
		}
	}

	// Convert epoch ms to DBMarlin date format: 2026-03-25+11:56:11
	fromTime := time.UnixMilli(fromMs).UTC().Format("2006-01-02+15:04:05")
	toTime := time.UnixMilli(toMs).UTC().Format("2006-01-02+15:04:05")

	dbmarlinURL := fmt.Sprintf(
		"http://34.69.236.9:9090/archiver/rest/v1/activity/summary?from=%s&to=%s&tz=Europe/London&interval=0&id=1",
		fromTime, toTime,
	)

	resp, err := http.Get(dbmarlinURL)
	if err != nil {
		log.Printf("DBMarlin proxy error: %v", err)
		writeError(w, http.StatusBadGateway, "failed to reach DBMarlin")
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// GET /dbmarlin-metrics?from=START_EPOCH_MS&to=END_EPOCH_MS
// Proxy endpoint — Harness CV passes epoch ms, DBMarlin expects date string format.
// This translates between the two and returns real DB wait time metrics from DBMarlin.
// Harness Custom Health Source config:
//
//	Base URL: http://35.192.158.3:8080
//	Path:     dbmarlin-metrics?from=start_time&to=end_time
//	Unit:     Milliseconds
func (h *Handler) DBMarlinMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	if fromStr == "" || toStr == "" {
		writeError(w, http.StatusBadRequest, "from and to parameters are required")
		return
	}

	// Harness passes epoch milliseconds — convert to DBMarlin date format
	fromMs, err := strconv.ParseInt(fromStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid from timestamp")
		return
	}
	toMs, err := strconv.ParseInt(toStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid to timestamp")
		return
	}

	// DBMarlin expects: 2026-03-25+11:56:11
	from := time.UnixMilli(fromMs).UTC().Format("2006-01-02+15:04:05")
	to := time.UnixMilli(toMs).UTC().Format("2006-01-02+15:04:05")

	dbmarlinURL := fmt.Sprintf(
		"http://34.69.236.9:9090/archiver/rest/v1/activity/summary?from=%s&to=%s&tz=Europe/London&interval=0&id=1",
		from, to,
	)

	log.Printf("DBMarlin proxy: %s", dbmarlinURL)

	resp, err := http.Get(dbmarlinURL)
	if err != nil {
		log.Printf("DBMarlin proxy error: %v", err)
		writeError(w, http.StatusBadGateway, "failed to reach DBMarlin")
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
