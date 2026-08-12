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
	// Secure by default (production sits behind a TLS-terminating proxy); set
	// COOKIE_SECURE=false for local plain-HTTP dev, where a Secure cookie would never
	// be sent back by the browser.
	cookieSecure := os.Getenv("COOKIE_SECURE") != "false"

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

	mux := web.NewMux(buildSHA, staticFS, db, cookieSecure)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("cladex listening on %s (build %s)", addr, buildSHA)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
