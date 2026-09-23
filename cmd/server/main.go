package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	cladex "github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/cli"
	"github.com/gerrygoo/cladex-web/internal/guia"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/web"
)

// buildSHA is set at build time via -ldflags "-X main.buildSHA=...".
var buildSHA = "dev"

// newLogger builds the process logger. Output goes to stdout, which is where the
// container runtime collects it: `docker logs cladex`, with rotation configured by the
// json-file driver options in compose.yaml. No log shipper, no agent — at this scale
// that stack would cost more memory than the app.
//
// LOG_FORMAT=text gives human-readable lines for local dev; the default is JSON so the
// deployed logs stay greppable with jq. LOG_LEVEL=debug surfaces static-asset and
// healthcheck requests, which are otherwise filtered out as noise.
func newLogger() *slog.Logger {
	level := slog.LevelInfo
	if err := level.UnmarshalText([]byte(strings.ToLower(os.Getenv("LOG_LEVEL")))); err != nil {
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if strings.EqualFold(os.Getenv("LOG_FORMAT"), "text") {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}

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
	// Writes made here are audited as coming from the CLI: there is no session user
	// behind `docker compose exec cladex /cladex user ...`.
	ctx := store.WithActor(context.Background(), store.Actor{Source: store.SourceCLI})
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
	logger := newLogger()
	slog.SetDefault(logger)

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
		logger.Error("open store", slog.Any("err", err))
		os.Exit(1)
	}
	defer db.Close()

	staticFS, err := fs.Sub(cladex.StaticFS, "static")
	if err != nil {
		logger.Error("static assets", slog.Any("err", err))
		os.Exit(1)
	}

	guiaFS, err := fs.Sub(cladex.GuiaFS, "docs/guia")
	if err != nil {
		logger.Error("guia assets", slog.Any("err", err))
		os.Exit(1)
	}
	guide, err := guia.New(guiaFS)
	if err != nil {
		logger.Error("render guia", slog.Any("err", err))
		os.Exit(1)
	}

	mux := web.NewMux(buildSHA, staticFS, guide, db, cookieSecure, logger)

	addr := fmt.Sprintf(":%s", port)
	logger.Info("listening", slog.String("addr", addr), slog.String("build", buildSHA))
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error("serve", slog.Any("err", err))
		os.Exit(1)
	}
}
