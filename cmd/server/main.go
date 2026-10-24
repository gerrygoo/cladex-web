package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // the home page shows the build time in Mexico City time; don't depend on the image's zoneinfo

	cladex "github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/appenv"
	"github.com/gerrygoo/cladex-web/internal/cli"
	factsvc "github.com/gerrygoo/cladex-web/internal/facturacion"
	"github.com/gerrygoo/cladex-web/internal/facturoporti"
	"github.com/gerrygoo/cladex-web/internal/guia"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/web"
)

// buildSHA and buildTime are set at build time via
// -ldflags "-X main.buildSHA=... -X main.buildTime=...". buildTime is RFC 3339 UTC;
// it stays empty for a plain `go run`, which the home page shows as a dev build.
var (
	buildSHA  = "dev"
	buildTime = ""
)

// newLogger builds the process logger. Output goes to stdout, which is where the
// container runtime collects it: `docker compose logs cladex-blue cladex-green`, with
// rotation configured by the json-file driver options in compose.yaml. No log shipper,
// no agent — at this scale that stack would cost more memory than the app.
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
// image ships a single binary (docker compose run --rm cladex user add ...), so
// user provisioning has to live behind the same entrypoint as the HTTP server.
func main() {
	if len(os.Args) > 1 && os.Args[1] == "user" {
		runCLI()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		runMigrateCLI(os.Args[2:])
		return
	}
	runServer()
}

// runMigrateCLI answers deploy.sh's question to a new image before it rolls production
// onto it: `cladex migrate pending-exclusive` lists, one per line, this build's
// migrations that the database hasn't applied and that are marked -- cladex:exclusive,
// without migrating anything. Exit 0: none, roll blue/green as usual; 3: some, so pause
// production for them; 1: couldn't tell.
func runMigrateCLI(args []string) {
	if len(args) != 1 || args[0] != "pending-exclusive" {
		fmt.Fprintln(os.Stderr, "usage: cladex migrate pending-exclusive")
		os.Exit(1)
	}
	names, err := store.PendingExclusive(context.Background(), dbPath(), cladex.MigrationsFS)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	for _, name := range names {
		fmt.Println(name)
	}
	if len(names) > 0 {
		os.Exit(3)
	}
}

func runCLI() {
	// Writes made here are audited as coming from the CLI: there is no session user
	// behind `docker compose run --rm cladex user ...`.
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

	// ctx is cancelled on SIGTERM (docker compose stop) or Ctrl-C, which starts a
	// graceful shutdown below.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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

	env, err := appenv.Parse(os.Getenv("APP_ENV"))
	if err != nil {
		logger.Error("environment", slog.Any("err", err))
		os.Exit(1)
	}
	// Facturación stays off, not fatal, when its secrets are missing or don't check out:
	// the rest of the app doesn't need them. The status says why, here and in /ajustes.
	factCfg, facturacion := facturoporti.Load(env, os.Getenv, time.Now())
	logger.Info("facturacion", slog.String("env", env.Name), slog.Bool("enabled", facturacion.Enabled),
		slog.Bool("sandbox", facturacion.Sandbox), slog.String("problem", facturacion.Problem),
		slog.String("rfc", facturacion.RFC), slog.Time("csd_expires", facturacion.Expires))

	build := web.Build{SHA: buildSHA, Env: env, Facturacion: facturacion}
	if t, err := time.Parse(time.RFC3339, buildTime); err == nil {
		build.Time = t
	}

	mux := web.NewMux(build, staticFS, guide, db, cookieSecure, logger, factsvc.New(db, factCfg, facturacion))

	// ReadHeaderTimeout stops a client holding a connection open by trickling headers.
	// ReadTimeout is generous because it covers the body too, and uploads can be large.
	// There's no WriteTimeout: it would cut off PDF rendering and PAC calls mid-response.
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%s", port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("listening", slog.String("addr", srv.Addr), slog.String("build", buildSHA), slog.String("built", buildTime))
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		logger.Error("serve", slog.Any("err", err))
		db.Close()
		os.Exit(1)
	case <-ctx.Done():
	}

	// Let in-flight requests finish before the deferred db.Close checkpoints the WAL.
	// Docker sends SIGKILL 10s after SIGTERM, so stop waiting before then.
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", slog.Any("err", err))
	}
}
