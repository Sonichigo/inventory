package main

type InventoryItem struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
	Location string `json:"location"`
	Unit     string `json:"unit"`
}

type Location struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	City string `json:"city"`
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