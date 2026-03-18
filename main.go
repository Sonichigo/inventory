package main

import (
	"log"
	"net/http"
	"os"
)

// ── Hardcoded config ──────────────────────────────────────────────────────────
// Dev  → DB_SERVER defaults to "localhost"
// Prod → set DB_SERVER=postgres (K8s Service name) to override

func buildConfig() Config {
	server := os.Getenv("DB_SERVER")
	if server == "" {
		server = "localhost" // dev default
	}

	return Config{
		InitUser: "user",
		InitPass: "password",
		Server:   server, // postgres-dbops in K8s, localhost for dev
		Port:     "5432",
		User:     "user",
		Password: "password",
		DBName:   "mydatabase",
	}
}

func main() {
	cfg := buildConfig()

	log.Printf("Connecting to Postgres at %s:%s db=%s", cfg.Server, cfg.Port, cfg.DBName)

	database, err := NewDB(cfg)
	if err != nil {
		log.Fatalf("Failed to initialise database: %v", err)
	}

	mux := http.NewServeMux()
	h := NewHandler(database)
	h.RegisterRoutes(mux)

	addr := ":8080"
	log.Printf("BBQBookkeeper listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
