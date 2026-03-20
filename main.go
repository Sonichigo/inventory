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
	return Config{
		InitUser: "user",
		InitPass: "password",
		Server:   server,
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

	mux.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.Dir("./ui"))))
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
