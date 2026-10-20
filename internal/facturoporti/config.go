// Package facturoporti is the app's side of Facturoporti, the PAC that stamps Cladex's
// CFDIs (docs/planning/facturas-timbrado-y-envio.md §8). This file loads its secrets.
//
// Secrets reach the container as files, Compose secrets mounted under /run/secrets (see
// compose.yaml and docs/NAS_OPERATIONS.md), never as environment variables, which
// `docker inspect` shows. Each path can be overridden with a *_FILE variable for local
// runs.
package facturoporti

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web/internal/appenv"
)

// The two Facturoporti hosts. Production stamps real CFDIs; the sandbox's stamps are not
// valid and are free.
const (
	ProductionURL = "https://api.facturoporti.com.mx"
	SandboxURL    = "https://testapi.facturoporti.com.mx"
)

// Secret files, by the variable that overrides each path and its default.
var secretFiles = []struct{ env, path, label string }{
	{"FACTUROPORTI_TOKEN_FILE", "/run/secrets/facturoporti_token", "token de Facturoporti"},
	{"CSD_CER_FILE", "/run/secrets/csd_cer", "certificado (.cer)"},
	{"CSD_KEY_FILE", "/run/secrets/csd_key", "llave privada (.key)"},
	{"CSD_PASSWORD_FILE", "/run/secrets/csd_password", "contraseña de la llave"},
}

// Config is everything a stamp request needs from outside the database.
type Config struct {
	BaseURL string
	Sandbox bool
	Token   string
	CSD     *CSD
}

// Status says whether facturación can run and, if not, why, for the logs at startup and
// for Configuración del sistema. It never carries a secret.
type Status struct {
	Enabled bool
	Sandbox bool
	Problem string // why it is off; empty when Enabled
	RFC     string // the CSD's, when it could be read
	Number  string
	Expires time.Time
}

// Load reads the secret files and checks the CSD at now. With none of the files present,
// facturación is simply not configured; with some but not all, or with a CSD that doesn't
// check out, it is off and Status.Problem says why. Only an explicitly production
// instance talks to the production host; every other one uses the sandbox, whatever its
// secrets are.
func Load(env appenv.Env, getenv func(string) string, now time.Time) (*Config, Status) {
	cfg := &Config{BaseURL: SandboxURL, Sandbox: true}
	if env.IsExplicitProduction() {
		cfg.BaseURL, cfg.Sandbox = ProductionURL, false
	}
	st := Status{Sandbox: cfg.Sandbox}

	values := make([][]byte, len(secretFiles))
	var missing []string
	for i, f := range secretFiles {
		path := getenv(f.env)
		if path == "" {
			path = f.path
		}
		b, err := os.ReadFile(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			missing = append(missing, f.label)
		case err != nil:
			st.Problem = fmt.Sprintf("no se pudo leer el %s: %v", f.label, err)
			return nil, st
		default:
			values[i] = b
		}
	}
	if len(missing) == len(secretFiles) {
		st.Problem = "no está configurada"
		return nil, st
	}
	// The CSD is checked whenever its three files are there, so the status shows whose it
	// is and whether it opens even while the token is still missing.
	var csd *CSD
	if values[1] != nil && values[2] != nil && values[3] != nil {
		// A password file written with echo ends in a newline that isn't part of it.
		password := strings.TrimRight(string(values[3]), "\r\n")
		var err error
		if csd, err = ParseCSD(values[1], values[2], password); err != nil {
			st.Problem = err.Error()
			return nil, st
		}
		st.RFC, st.Number, st.Expires = csd.RFC, csd.Number, csd.NotAfter
	}
	if len(missing) > 0 {
		st.Problem = "falta: " + strings.Join(missing, ", ")
		return nil, st
	}
	if cfg.Token = strings.TrimSpace(string(values[0])); cfg.Token == "" {
		st.Problem = "el token de Facturoporti está vacío"
		return nil, st
	}
	if err := csd.CheckValidAt(now); err != nil {
		st.Problem = err.Error()
		return nil, st
	}
	cfg.CSD = csd
	st.Enabled = true
	return cfg, st
}
