# Cladex — Testing

How this project is tested, what the coverage actually is, and which gaps are
tracked but not yet closed. Companion to `docs/PLAN.md`, which records *why* each
slice was built the way it was; this file records *how it is verified*.

## Running the tests

```bash
go test ./...
```

Prerequisite: the **`typst` CLI must be on `PATH`**. `internal/web`'s PDF, Emitir,
and Revisar tests shell out to it through `internal/pdf.Render` and fail — they do
not skip — when it is missing. On the dev Mac it comes from Homebrew
(`brew install typst`); in the deployed image it is the static musl binary.

Coverage, per package and as a single cross-package number:

```bash
go test ./... -cover
```

```bash
go test ./... -coverpkg=./... -coverprofile=/tmp/cover.out && go tool cover -func=/tmp/cover.out | tail -1
```

To see which functions in a package are untested, sorted worst-first:

```bash
go tool cover -func=/tmp/cover.out | grep -v '100.0%' | sort -k3 -n
```

## Conventions

- **Real SQLite, real migrations, no mocks.** Store and handler tests open a fresh
  file DB in `t.TempDir()` via `store.Open(ctx, dsn, cladex.MigrationsFS)` and
  register `t.Cleanup(s.Close)`. See `newTestStore` in `internal/store/auth_test.go`
  and `newTestAuth` in `internal/web/auth_test.go`. A test therefore also exercises
  the migration chain, which is deliberate — schema drift shows up as a test
  failure, not a production surprise.
- **Handlers are tested through `httptest`**, constructed directly
  (`NewProducts(s)`, `NewQuotes(s, dir)`) and, where the route is gated, wrapped by
  hand in `a.RequireAuth(a.RequireAdmin(...))`. This is fast, but it is also the
  source of the router gap tracked below: the test wraps the middleware, so it
  proves the middleware works, not that `NewMux` applies it.
- **bcrypt at `MinCost` in tests.** Production provisioning uses cost 12; tests use
  `bcrypt.MinCost` so the suite stays under a couple of seconds.
- **Money and pricing are held to a higher bar than everything else.** Prices are
  computed deterministically in code and never touched by a model
  (`docs/PLAN.md`, pinned constraint 1), so `internal/pricing` and `internal/money`
  are expected to stay at or near 100%. Treat a drop there as a release blocker;
  treat a drop in a CRUD handler as a normal bug.
- **The generated `internal/views/*_templ.go` files are not tested directly.** They
  are covered incidentally by the handler tests (~58% of their statements). Do not
  write tests against generated code; test the handler that renders it.

## Coverage snapshot — 2026-09-22 (at `05e9567`)

| Package | Coverage | Notes |
|---|---|---|
| `internal/pricing` | 100.0% | The quoting engine. Keep it here. |
| `internal/money` | 99.0% | |
| `internal/pdf` | 94.2% | |
| `internal/store` | 79.5% | Remainder is mostly `if err != nil` branches. |
| `internal/cli` | 76.4% | |
| `internal/web` | 60.3% | Handlers; where the real gaps are. |
| `cmd/import` | 18.7% | One of three spreadsheet parsers tested. |
| `cmd/server`, `cmd/cladexctl` | 0.0% | Startup and flag wiring. |
| `internal/views` | 0.0% standalone / ~58% via handler tests | Generated templ code. |

Cross-package total, counting generated views and the `main` packages: **57.0%**.
All tests pass; nothing is skipped or marked `t.Skip`.

## Tracked gaps

Ordered by how much a defect there would cost. Tick one off in the same commit that
closes it, and refresh the snapshot above when the numbers move.

- [ ] **Route permissions are never asserted.** `NewMux` is at 0%. Handler tests
      apply `RequireAuth`/`RequireAdmin` themselves, so deleting `RequireAdmin` from
      `/usuarios`, `/ajustes`, or `/unidades` in `internal/web/router.go` breaks
      nothing in the suite. The fix is one table-driven test that builds the real
      mux and walks every route twice — once signed out, once as a vendedor —
      asserting a login redirect and a 403 respectively. This also drags most of the
      next item into coverage.
- [ ] **Handlers with no test at all:** all of `internal/web/units.go`;
      `products.CreateConversion` / `DeleteConversion`; `quotes.List`;
      `friendlyPricingError`; the `NewPage` handlers; `auth.LoginPage`.
- [ ] **Quote create and issue are happy-path only.** `quotes.Create` 43.5%,
      `quotes.Emitir` 64.8%. Validation failures and the error branches around folio
      assignment and PDF writing are untested — and these are the routes that mint
      the artifact the customer actually receives.
- [ ] **Untested store functions:** `DisableUser`, `DeleteSessionsByUserID`,
      `QuoteCount`. `DeleteSessionsByUserID` is the one that matters: it is what
      ends a disabled or password-reset user's live sessions.
- [ ] **Two of three import parsers untested:** `parseABASTILUM` and `parseCCA` are
      at 0%; only `parseCCSAC` has a test. Import is a one-shot operation per
      catalog, which is why this ranks below the others, but a silent misparse puts
      wrong prices in the DB.
- [ ] **CI does not run the tests.** `.github/workflows/deploy.yml` builds and pushes
      to GHCR with no `go test ./...` step, so a red suite still deploys. Add the
      step before the build.

## Deliberate non-goals

- No browser/E2E automation. Slices are verified by hand in a real browser and that
  verification is written into the slice's notes in `docs/PLAN.md` — for a <5-user
  app that is the right trade.
- No mock or fake `Store` interface. Tests hit real SQLite; see Conventions.
- No coverage threshold gate. The checklist above is the tracking mechanism.
