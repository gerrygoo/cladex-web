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

CI runs exactly this (plus `go vet ./...`) in the `test` job of
`.github/workflows/deploy.yml`, and `build-push` depends on it — a failing test
blocks the deploy.

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
  (`NewProducts(s)`, `NewQuotes(s, logger)`) and, where the route is gated, wrapped by
  hand in `a.RequireAuth(a.RequireAdmin(...))`. This is fast, but it proves only that
  the middleware works — that `NewMux` actually applies it is `router_test.go`'s job.
- **Every route in `NewMux` has a row in `router_test.go`'s table.** That table is what
  proves the router applies `RequireAuth`/`RequireAdmin` at all; a new route without a
  row is an untested gate. Adding the row is part of adding the route.
- **Everything `docs/guia/` quotes is pinned by a test.** The user guide names buttons,
  fields, and messages exactly as they appear on screen, so a reworded message silently
  makes the guide wrong. `internal/web/guia_test.go` asserts those exact strings, grouped
  by the guide page and section they back. Change a message → change the guide → change
  the test, in one commit.
- **bcrypt at `MinCost` in tests.** Production provisioning uses cost 12; tests use
  `bcrypt.MinCost` so the suite stays under a couple of seconds.
- **Money and pricing are held to a higher bar than everything else.** Prices are
  computed deterministically in code and never touched by a model
  (`docs/PLAN.md`, pinned constraint 1), so `internal/pricing` and `internal/money`
  are expected to stay at or near 100%. Treat a drop there as a release blocker;
  treat a drop in a CRUD handler as a normal bug.
- **The generated `internal/views/*_templ.go` files are not tested directly.** They
  are covered incidentally by the handler tests (~82% of their statements). Do not
  write tests against generated code; test the handler that renders it.

## Coverage snapshot — 2026-09-28 (after M3 and the guide-conformance pass)

| Package | Coverage | Notes |
|---|---|---|
| `internal/pricing` | 100.0% | The quoting engine. Keep it here. |
| `internal/money` | 99.0% | |
| `internal/pdf` | 94.4% | |
| `internal/guia` | 89.6% | The in-app guide renderer. |
| `internal/store` | 80.2% | Remainder is mostly `if err != nil` branches. |
| `internal/cli` | 76.4% | |
| `internal/web` | 79.3% | Includes the router gating and guide-conformance tests. |
| `cmd/server`, `cmd/cladexctl` | 0.0% | Startup and flag wiring. |
| `internal/views` | 18.0% standalone / ~82% via handler tests | Generated templ code. |

Cross-package total, counting generated views and the `main` packages: **70.5%**
(57.0% → 67.2% → 69.5% over the two gap-closing passes, before M3 deleted `cmd/import`
and the FX/copper/flat-price paths). All tests pass; nothing is skipped.

## Tracked gaps

Ordered by how much a defect there would cost. Tick one off in the same commit that
closes it, and refresh the snapshot above when the numbers move.

- [x] **Route permissions are never asserted.** Closed by `internal/web/router_test.go`,
      which builds the real mux and walks a hand-maintained route table three ways:
      signed out (every route must bounce to `/login`), as a vendedor (admin routes must
      403, ordinary routes must not), and as an admin (admin routes must not 403). Both
      directions are asserted, so over-gating an ordinary route fails too. Verified by
      mutation: dropping `RequireAdmin` from `/unidades` and `RequireAuth` from
      `/clientes` each fail the suite. **Add a route to `NewMux` → add it to the table.**
- [x] **Handlers with no test at all.** `internal/web/units_test.go` covers the
      `/unidades` list, create, duplicate-code, and empty-field paths;
      `friendlyPricingError` has a table test pinning pricing's errors (bare and
      wrapped) to their Spanish messages; the rest (`quotes.List`, the `NewPage` handlers, `LoginPage`)
      are now exercised by the router tests.
- [x] **Untested store functions.** `DisableUser` and `DeleteSessionsByUserID` are
      covered in `internal/store/auth_test.go`. (`QuoteCount` was covered too until the
      home page's pipeline overview replaced it.)
      The session tests assert the parts that matter operationally: disabling a user
      kills their live cookie through `SessionUser`, and a password reset ends *every*
      session that user has while leaving other users' sessions alone.
- [x] **Two of three import parsers untested.** Closed, then moot: M3 deleted
      `cmd/import` along with the spreadsheet-shaped pricing fields it fed.
- [x] **CI does not run the tests.** `.github/workflows/deploy.yml` gained a `test` job
      (`go vet ./...` then `go test ./...`, with the pinned typst binary installed) that
      `build-push` now `needs`, so a red suite blocks the deploy.
- [ ] **Quote create and issue still lack their error paths.** `quotes.Create` 73.9% and
      `quotes.Emitir` 76.5% — the validation paths are covered now, but the store-failure
      branches are not reachable without fault injection, which would mean introducing a
      `Store` interface purely for tests. Deliberately left open; revisit only if these
      branches ever misbehave in production.

## Guide conformance

`docs/guia/` is written for sellers and quotes the UI verbatim, which makes it a
specification an automated test can hold the app to. `internal/web/guia_test.go` covers,
per guide page:

- **acceso.md** — the rate-limit wait message; the three `Mi cuenta` password errors
  (missing current password, shorter than 8 characters, mismatched confirmation).
- **README.md** — the role table: a vendedor's menu has no Usuarios/Unidades/Ajustes
  links, an admin's has all three.
- **clientes.md** — `Cliente guardado.`, `El nombre es obligatorio.`, `Email inválido.`,
  and the delete rules (gone from the list and from the new-quote customer picker, while
  their existing quotes stay listed).
- **productos.md** — `Producto guardado.`, the five form validation messages including
  the Costo and Peso variants, the two Materiales messages (non-positive quantity,
  duplicate material), and the Costo column's three forms (cost / `+ materiales` /
  `materiales`).
- **cotizaciones.md** — `Selecciona un cliente.` / `Selecciona una serie de folio.`, the
  three line-level errors, `Borrador guardado.`, `Cotización emitida.`, `Producto no encontrado.` on a draft whose product was deleted (and that
  such a draft refuses to save), the 30-day vigencia, that an empty quote does not issue,
  and the 409s on an issued quote.

Already covered elsewhere and deliberately not duplicated here: the pricing formulas
(`internal/pricing`), revision numbering and the "only the latest issued can be revised"
rule (`internal/store/quotes_test.go`), issue-time immutability and byte-identical
reprints (`TestQuotesEmitir`, `TestQuotesIssuedPageShowsFrozenLines`), customer search
across name/RFC/contact, and the admin self-lockout guards.

**One discrepancy found, left open as a product call:** the guide's troubleshooting table
documents *esta cotización ya no es editable* for acting on an already-issued quote.
`Guardar` and `Recalcular` produce exactly that, but `Emitir` answers the same situation
with *esta cotización ya fue emitida*. Both are 409s meaning the same thing; only the
first is in the guide. The test asserts the status for all three and the wording for the
two that match. Fix by either unifying the message in `internal/web/quotes.go` or adding
the second wording to the guide.

## Deliberate non-goals

- No browser/E2E automation. Slices are verified by hand in a real browser and that
  verification is written into the slice's notes in `docs/PLAN.md` — for a <5-user
  app that is the right trade.
- No mock or fake `Store` interface. Tests hit real SQLite; see Conventions.
- No coverage threshold gate. The checklist above is the tracking mechanism.
