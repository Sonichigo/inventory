// ── Hardcoded config ──────────────────────────────────────────────────────────
// Dev  → DB_SERVER defaults to "localhost"
// Prod → set DB_SERVER=postgres (K8s Service name) to override

package main

import (
	"log"
	"net/http"
	"os"
)

func buildConfig() Config {
	server := os.Getenv("DB_SERVER")
	if server == "" {
		server = "localhost"
	}
	sqlDir := os.Getenv("SQL_DIR")
	if sqlDir == "" {
		sqlDir = "./sql"
	}
	return Config{
		InitUser: "user",
		InitPass: "password",
		Server:   server,
		Port:     "5432",
		User:     "user",
		Password: "password",
		DBName:   "mydatabase",
		SQLDir:   sqlDir,
	}
}

func main() {
	cfg := buildConfig()
	log.Printf("Connecting to Postgres at %s:%s db=%s sqlDir=%s",
		cfg.Server, cfg.Port, cfg.DBName, cfg.SQLDir)

	database, err := NewDB(cfg)
	if err != nil {
		log.Fatalf("Failed to initialise database: %v", err)
	}

	mux := http.NewServeMux()
	h := NewHandler(database)
	h.RegisterRoutes(mux)

	// Serve demo UI at /ui/
	mux.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.Dir("./ui"))))
	// Redirect root to UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/ui/", http.StatusFound)
		}
	})

	addr := ":8080"
	log.Printf("BBQBookkeeper listening on %s — UI at http://localhost%s/ui/", addr, addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
