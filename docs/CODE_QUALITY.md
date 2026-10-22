# Code quality audit

Findings from a code quality audit of the Go codebase on 2026-10-22, against commit
`35625e7`. Each finding is a checkbox: tick it off (and note the commit) when it's fixed.
Line numbers are as of that commit; function names are the stable reference.

Baseline at audit time: `go build` and `go vet` clean, `go test ./...` passing,
templ-generated files in sync with their `.templ` sources.

## High

- [x] **H1. Foreign keys are off on most database connections.** Fixed in `6b28864`. Still to do: run `PRAGMA foreign_key_check` on a copy of production.
  `store.Open` (`internal/store/store.go:29`) runs `PRAGMA foreign_keys = ON; PRAGMA
  journal_mode = WAL;` once through the `sql.DB` pool. `foreign_keys` is per connection, so
  only the connection that ran it enforces them; every other pooled connection has them
  off. A probe opening 20 concurrent transactions saw 18 connections with
  `foreign_keys = 0`, and `busy_timeout = 0` on all 20.
  - Impact: the 18 `ON DELETE` clauses in `migrations/` and FK integrity in general hold
    only intermittently, so production likely has orphan rows. With no busy timeout, two
    concurrent writers get an immediate `SQLITE_BUSY`, which surfaces as a 500. Deferred
    transactions that read and then write can also fail to upgrade their lock under WAL.
  - Fix: set the pragmas in the DSN so every connection gets them, e.g.
    `?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate`.
    The FK-off migration path in `applyMigration` keeps working, since it flips the
    pragma on its own connection. Then run `PRAGMA foreign_key_check` against a copy of
    the production database and clean up any orphans found.

- [x] **H2. Internal errors are never logged.** Fixed in `4c24e8f`. `internal/web` has 132
  `http.Error(w, "error interno", 500)` call sites that discard `err`. The access log shows
  the 500 but not why. Only `facturas.go` logs its errors.
  - Fix: a `serverError(w, r, err)` helper that logs `err` with the request's
    `request_id` (from `requestInfoFrom`) and writes the 500, used at every site.

- [x] **H3. HTTP server has no timeouts and no graceful shutdown.** Fixed in `b06c468`.
  `cmd/server/main.go:158` uses bare `http.ListenAndServe`: no `ReadHeaderTimeout`
  (slowloris), and on SIGTERM the process dies without running `defer db.Close()`, so
  in-flight writes are cut and the WAL isn't checkpointed before Litestream sees it.
  - Fix: an `http.Server` with `ReadHeaderTimeout`/`ReadTimeout`/`IdleTimeout`, plus
    `signal.NotifyContext` and `srv.Shutdown(ctx)` before closing the store.

- [ ] **H4. Login IP rate limit is bypassable.** `clientIP` (`internal/web/auth.go`) trusts
  the leftmost `X-Forwarded-For` entry, which the client controls. Rotating it evades the
  per-IP throttle (the per-username throttle still applies, so this mostly enables
  password spraying across usernames).
  - Fix: use the address the proxy appends (rightmost entry) or a header the proxy sets
    and overwrites, such as `X-Real-IP`.
  - Blocked on the proxy chain (nginx on the VPS, then FRP's HTTP vhost): which entry is
    trustworthy depends on whether FRP appends to `X-Forwarded-For` after nginx does. If
    it does, the rightmost entry is nginx's own address and every client would share one
    throttle bucket. Check the VPS's nginx site config and FRP before changing it.

## Medium

- [x] **M1. Public leftover scaffolding routes.** Fixed in `4efe382`. `GET /saludo` and `GET /sample.pdf`
  (`internal/web/router.go:192-205`) are unauthenticated. `/sample.pdf` spawns a `typst`
  process per request and echoes typst's stderr to the client. Delete both, along with
  `sampleTypst`. More generally, nothing bounds the number of concurrent `typst`
  processes (`internal/pdf/typst.go`); a small semaphore in `pdf.Render` would.

- [ ] **M2. Pricing logic lives in the HTTP layer.** `internal/web/quotes.go` is ~1,500
  lines. `resolveMargin` (126 lines) and `computeQuoteLines` (130 lines) hold pricing
  rules that belong in `internal/pricing` / `internal/store`, where they'd be unit-testable
  without HTTP.
  - `computeQuoteLines` does 2 queries per line (`ProductByID` + `ListProductMaterials`)
    on every recalculation: 2N queries per keystroke for an N-line quote. Load products
    and materials for all lines in one query each.
  - It reports any store error as "Producto no encontrado." / "Error al cargar el
    producto.", hiding real database failures. Distinguish not-found from errors.

- [x] **M3. `Emitir` writes in three separate transactions.** Fixed in `4eac29c`. `ReplaceQuoteLines`,
  `saveTermsInputs` and `IssueQuote` each commit on their own. `IssueQuote`'s
  `status = 'borrador'` guard keeps it correct, but a double-submit returns a 500 because
  `ErrQuoteNotDraft` isn't mapped to a 409.

- [x] **M4. Go toolchain has a known vulnerability.** Fixed in `b251e80`. govulncheck reports GO-2026-6809
  (`crypto/x509`, reachable) on Go 1.26.9; fixed in 1.26.10. Bump the `go` directive in
  `go.mod` and rebuild the image.

## Low / hygiene

- [x] **L1. staticcheck findings.** Fixed in `136d821`.
  - `internal/views/projects.go:69`: `joinNonEmpty` is unused (U1000).
  - `internal/cli/cli.go:145,155`: `fmt.Errorf(usage)` with a non-constant format
    string (SA1006); use `errors.New(usage)`.
  - `internal/cli/cli.go:27`: error string ends with punctuation (ST1005).
- [x] **L2. Duplicated file serving.** Fixed in `3028629`. `Projects.Archivo` (`internal/web/projects.go:~500`)
  re-implements `serveUploadedFile` (`internal/web/documentos.go:168`) instead of calling
  it.
- [x] **L3. Inconsistent route wrapping.** Fixed in `228379a`. `router.go` mixes the `admin(...)` helper with
  hand-nested `auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(...)))`. Use the helper
  throughout (and a matching `authed(...)`).
- [x] **L4. Migration numbering and a stale reference.** Fixed in `cae6bd9`. Two migrations share `0004`
  (`0004_add_customer_postal_code_and_tax_regime.sql`, `0004_audit_log.sql`). Harmless
  (ordering is by full filename) but confusing; don't renumber applied files, just avoid
  it going forward. `RequireAuth`'s comment in `internal/web/auth.go:101` points to
  `migrations/0003_audit_log.sql`; it's `0004_audit_log.sql`.
- [x] **L5. No global security headers.** Fixed in `376d82b`. Nothing sets `X-Frame-Options: DENY` or CSP
  `frame-ancestors 'none'`, so pages can be framed (clickjacking). Add them in a
  middleware next to the CSRF check.
- [x] **L6. Stringly-typed roles and statuses.** Fixed in `ec96816`. `"admin"`, `"borrador"`, `"emitida"`,
  `"revisada"` are repeated as literals across `store`, `web` and `views` (~25 non-test
  sites). Named constants in `store` would catch typos at compile time.
- [x] **L7. Mutable package global.** Fixed in `9deea03`. `NewMux` assigns `views.EnvironmentBanner`, a package
  variable; two muxes in one process (e.g. parallel tests) race on it. Pass it through
  the layout's view model instead.
- [x] **L8. CI only runs `vet` and `test`.** Fixed in `997b31e`. Add to `.github/workflows/deploy.yml`:
  `staticcheck`, `govulncheck`, and a templ drift check (`templ generate` then
  `git diff --exit-code`).

## What's solid

Recorded so a future audit doesn't relitigate them:

- SQL is parameterized; `ORDER BY` columns go through the `orderByClause` whitelist.
- CSRF via `http.CrossOriginProtection`; session cookies `HttpOnly`, `Secure`, `SameSite=Lax`.
- bcrypt with a dummy-hash compare for unknown users (no timing enumeration); session
  tokens stored as SHA-256 hashes; exponential login backoff per username and per IP.
- Upload size limits via `MaxBytesReader`; uploads served with `nosniff` and a `sandbox` CSP.
- Typst input escaping, with a test (`TestRenderQuoteEscapesUntrustedContent`).
- Migration runner checks `foreign_key_check` after FK-off migrations and rolls back on
  violations.
- Audit-log triggers attribute every write to a user or the CLI.
- Store-level state guards (`WHERE status = 'borrador'`) make issuing idempotent.
- Extensive test coverage across `store` and `web`.
