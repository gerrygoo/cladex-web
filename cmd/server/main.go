package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	cladex "github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/web"
)

// buildSHA is set at build time via -ldflags "-X main.buildSHA=...".
var buildSHA = "dev"

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "data/cladex.db"
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Fatalf("data dir: %v", err)
	}

	ctx := context.Background()
	db, err := store.Open(ctx, dbPath, cladex.MigrationsFS)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer db.Close()

	staticFS, err := fs.Sub(cladex.StaticFS, "static")
	if err != nil {
		log.Fatalf("static assets: %v", err)
	}

	mux := web.NewMux(buildSHA, staticFS, db)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("cladex listening on %s (build %s)", addr, buildSHA)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
