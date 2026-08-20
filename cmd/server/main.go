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
	"github.com/gerrygoo/cladex-web/internal/cli"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/web"
)

// buildSHA is set at build time via -ldflags "-X main.buildSHA=...".
var buildSHA = "dev"

func dbPath() string {
	if p := os.Getenv("DB_PATH"); p != "" {
		return p
	}
	return "data/cladex.db"
}

func openStore(ctx context.Context) (*store.Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath()), 0o755); err != nil {
		return nil, fmt.Errorf("data dir: %w", err)
	}
	return store.Open(ctx, dbPath(), cladex.MigrationsFS)
}

// main dispatches to the admin CLI when invoked as `cladex user ...` — the deployed
// image ships a single binary (docker compose exec cladex /cladex user add ...), so
// user provisioning has to live behind the same entrypoint as the HTTP server.
func main() {
	if len(os.Args) > 1 && os.Args[1] == "user" {
		runCLI()
		return
	}
	runServer()
}

func runCLI() {
	ctx := context.Background()
	db, err := openStore(ctx)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer db.Close()

	if err := cli.Run(ctx, db, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runServer() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}
	// Secure by default (production sits behind a TLS-terminating proxy); set
	// COOKIE_SECURE=false for local plain-HTTP dev, where a Secure cookie would never
	// be sent back by the browser.
	cookieSecure := os.Getenv("COOKIE_SECURE") != "false"

	ctx := context.Background()
	db, err := openStore(ctx)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer db.Close()

	staticFS, err := fs.Sub(cladex.StaticFS, "static")
	if err != nil {
		log.Fatalf("static assets: %v", err)
	}

	mux := web.NewMux(buildSHA, staticFS, db, cookieSecure, filepath.Dir(dbPath()))

	addr := fmt.Sprintf(":%s", port)
	log.Printf("cladex listening on %s (build %s)", addr, buildSHA)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
