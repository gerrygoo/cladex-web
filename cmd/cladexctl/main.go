// Command cladexctl is a local-dev convenience wrapper around internal/cli — the
// deployed image ships only cmd/server's binary (as /cladex), which dispatches the
// same "user ..." subcommands directly; see cmd/server/main.go.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	cladex "github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/cli"
	"github.com/gerrygoo/cladex-web/internal/store"
)

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "data/cladex.db"
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Fatalf("data dir: %v", err)
	}

	ctx := store.WithActor(context.Background(), store.Actor{Source: store.SourceCLI})
	s, err := store.Open(ctx, dbPath, cladex.MigrationsFS)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer s.Close()

	if err := cli.Run(ctx, s, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
