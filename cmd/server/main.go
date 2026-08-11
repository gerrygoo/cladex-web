package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"

	cladex "github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/web"
)

// buildSHA is set at build time via -ldflags "-X main.buildSHA=...".
var buildSHA = "dev"

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	staticFS, err := fs.Sub(cladex.StaticFS, "static")
	if err != nil {
		log.Fatalf("static assets: %v", err)
	}

	mux := web.NewMux(buildSHA, staticFS)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("cladex listening on %s (build %s)", addr, buildSHA)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
