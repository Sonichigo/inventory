package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
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
	mux.HandleFunc("/locations", h.Locations)
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

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	if err := h.db.Ping(); err != nil {
		dbStatus = "unavailable: " + err.Error()
		writeJSON(w, http.StatusServiceUnavailable, HealthResponse{
			Status:   "degraded",
			Database: dbStatus,
		})
		return
	}
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok", Database: dbStatus})
}

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
	items, err := h.db.GetInventoryByLocation(location)
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

func (h *Handler) InventoryByID(w http.ResponseWriter, r *http.Request) {
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