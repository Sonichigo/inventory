package main

type InventoryItem struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
	Location string `json:"location"`
	Unit     string `json:"unit"`
	Supplier string `json:"supplier,omitempty"`
	LeadDays int    `json:"lead_days,omitempty"`
}

type Location struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	City string `json:"city"`
}

type Supplier struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
	Item     string `json:"item"`
	LeadDays int    `json:"lead_days"`
}

type SupplierSummary struct {
	Supplier    string  `json:"supplier"`
	Location    string  `json:"location"`
	ItemCount   int     `json:"item_count"`
	AvgLeadDays float64 `json:"avg_lead_days"`
	TotalStock  int     `json:"total_stock"`
}

type LowStockItem struct {
	Location string `json:"location"`
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
	Unit     string `json:"unit"`
}

type AddItemRequest struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
	Location string `json:"location"`
	Unit     string `json:"unit"`
}

type UpdateQuantityRequest struct {
	ID       int `json:"id"`
	Quantity int `json:"quantity"`
}

type HealthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}
