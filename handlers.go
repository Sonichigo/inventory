package main

import (
	"encoding/json"
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

// GET /metrics
// Option A CV endpoint — polled by Harness Custom Health Source every 30s.
// Returns rolling query timing stats for the hot /inventory-by-location query.
// Harness CV thresholds: avg_response_ms > 50 = degraded, > 100 = fail
func (h *Handler) Metrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, metrics.Snapshot())
}
