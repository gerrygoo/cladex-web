# Cladex — Stack, Deployment, and CRUD MVP

## Context

Cladex is a small electrical-cable and lighting distributor in Querétaro, MX (~3 sellers,
<5 users). They quote from an Excel workbook (`Cladex catalogo precios.xlsx`) that wires
four price-list sheets to four "Cotizador" quote forms with manual VLOOKUPs. This project
replaces it.

**The MVP is a CRUD web app with authentication**, over products, customers, users,
settings, and quotes. The quoting engine and PDF output follow. LLM-assisted ingestion of
inbound RFPs is explicitly **deprioritized to backlog** — it is a force multiplier on a
working quoting tool, not a prerequisite for one.

Two pinned constraints:
1. Prices are computed deterministically in code from the DB. No model output ever reaches
   a price field.
2. Quote lines snapshot price, description, and pricing inputs (kg/m, margin, FX) at issue
   time — copper and USD/MXN move constantly.

## Decisions (settled)

| | Choice | Why |
|---|---|---|
| Host | UGREEN NASync **DXP4800** (N100, 4 core, 8 GB), port **8090** | Existing infrastructure |
| Hostnames | `cotizador.cladex.com.mx` canonical; `c.cladex.com.mx` 301s to it | Company domain; shorthand mirrors the existing `jf.`/`js.` convention |
| Stack | **Go + templ + htmx** | Minimal deps, no npm churn, ~20 MB image, ~25 MB idle on a shared 8 GB box |
| DB | **SQLite** via `modernc.org/sqlite` (pure Go) | No cgo → static binary → distroless |
| PDF | **Typst** (static musl binary in image) | Chromium idles ~400 MB, spikes >1 GB/render — untenable beside Jellyfin |
| Registry | **Public GHCR** image | Repo is public; no auth needed on the NAS |
| Deploy | Actions build+push → NAS cron pulls every 5 min | NAS is behind NAT; the pull must be outbound |
| Auth | **Username + password, admin-provisioned** | No self-service surface; OAuth/2FA deferred |

## Environment

- Dev Mac: macOS 26.5.1, **arm64**. Container runtime and Go toolchain per slice 0.0a.
- Repo: `github.com/gerrygoo/cladex-web`, **public**.
- NAS: Docker Compose, `network_mode: host`, configs under `/volume1/docker/[service]/`,
  PUID **1000** / PGID **10**. Ports taken: 8080 (FRP), 8081, 8096, 8191, 8282, 5055, 7878, 8989.
- VPS: Hetzner Ubuntu, nginx + certbot, FRP server multiplexing 80/443.
- **SSH**: key-based, non-interactive access is set up to both boxes — `ssh cladex-nas`
  (192.168.3.169, user `ggo`, key at `~/.ssh/cladex_nas`, docker-group membership granted)
  and `ssh cladex-vps` (5.78.203.98, user `root`, key `cladex-vps` stored in 1Password's
  SSH agent). Both aliases live in `~/.ssh/config`. A **password prompt** (SSH or `sudo`)
  still can't be answered by a non-interactive shell — that's the one thing that still
  bounces back to the user (e.g. installing a crontab entry).

---

# Technical design — CRUD MVP

## Application structure

```
cmd/server/main.go          # config from env, open db, migrate, serve
cmd/cladexctl/main.go       # admin CLI: user add/passwd/disable
internal/web/               # router, middleware, handlers, form structs
internal/views/             # .templ files (layout, pages, partials)
internal/store/             # database/sql access, one file per entity
internal/money/             # fixed-point money type + es-MX formatting
internal/pdf/               # typst subprocess
migrations/                 # embedded .sql, applied at boot
static/                     # htmx.min.js, app.css — served from embed.FS
```

Stdlib `net/http` `ServeMux` with Go 1.22+ method patterns (`GET /productos/{id}`). No
router dependency. `internal/store` uses plain `database/sql` — the schema is ~9 tables and
hand-written scanning is tractable. If the boilerplate becomes painful, `sqlc` is the
escape hatch, but don't start there.

## Money — decide before writing any schema

**Never `float64`.** The catalog already contains values like `6.319872233629933` per
metre; centavos alone are too coarse for unit prices, and floats accumulate error across
line totals.

- Unit prices: `int64` **micro-pesos** (1e-6 MXN) — `unit_price_micros`.
- Line totals, subtotal, IVA, total: `int64` **centavos**.
- Rounding happens exactly once, at line total, half-up, and is then fixed.
- `internal/money` owns the type, the arithmetic, and es-MX display formatting
  (`$1,234.56`). Hand-rolled — `golang.org/x/text` is not worth a dep for one format.

This is the single most expensive thing to retrofit, which is why it precedes the schema.

## Auth design

**Provisioning is out-of-band.** No signup, no public password reset, no email dependency.

```
docker compose exec cladex /cladex user add rodolfo --name "Rodolfo Flores" --role vendedor
docker compose exec cladex /cladex user passwd rodolfo
docker compose exec cladex /cladex user disable emilio
```

`user add` generates a random initial password and prints it once. The distroless image has
**no shell**, so `exec ... sh` fails — exec the binary directly, as above.

**Hashing**: `golang.org/x/crypto/bcrypt`, cost 12. One dependency, Go-team maintained, no
tuning parameters to get wrong. (`scrypt` is *not* Go stdlib; it also lives in `x/crypto`.
Go 1.24's stdlib `crypto/pbkdf2` exists but is weaker against GPU attack for equal effort.)

**Sessions**: server-side, in SQLite. The cookie carries a 32-byte random token; the DB
stores only its SHA-256. Chosen over stateless signed cookies because it makes sessions
**revocable** and there is no signing secret to rotate.

Cookie flags: `HttpOnly`, `Secure`, `SameSite=Lax`, `Path=/`, 14-day expiry, sliding renewal.

**CSRF**: `SameSite=Lax` plus stdlib `http.CrossOriginProtection` (Go 1.25+), validating
`Sec-Fetch-Site`. Zero dependencies. Fall back to per-session form tokens on older Go.

**Login rate limiting**: this endpoint faces the open internet. Per-username and per-IP
attempt counters in SQLite, exponential backoff after 5 failures.

**In-app auth UI** is exactly three things: login, logout, change-own-password. A forgotten
password is an admin re-running `user passwd`.

**Roles**: `admin` and `vendedor`. Admin gates users and settings (FX rate, metal prices,
margins); vendedor gets everything else. Checked in middleware — not a permissions framework.

## Routes

| Route | Access | Notes |
|---|---|---|
| `GET/POST /login`, `POST /logout` | public | |
| `GET /` | auth | Dashboard: quote counts, recent quotes |
| `GET /productos`, `/productos/nuevo`, `GET/POST /productos/{id}` | auth | List + search + edit |
| `GET /clientes`, `/clientes/nuevo`, `GET/POST /clientes/{id}` | auth | |
| `GET /cotizaciones`, `GET /cotizaciones/{folio}` | auth | History; PDF re-download |
| `GET/POST /usuarios` | **admin** | |
| `GET/POST /ajustes` | **admin** | FX rate, metal prices, default margins |
| `GET /mi-cuenta` | auth | Change own password |
| `GET /healthz` | public | Build SHA |

## UI conventions

- **Spanish throughout.** The users are Mexican sellers; hardcode Spanish strings, no i18n
  framework.
- **Forms work without JS.** Standard POST + redirect. htmx enhances — live table search
  (`hx-get` into a `<tbody>`), delete confirmations, inline edits — but never gates.
- Validation: one struct per form with `Validate() map[string]string`; field errors render
  back into the same templ form with values preserved.
- Flash messages via the session.
- **User guide.** `docs/guia/` is the sellers' manual: Spanish, one page per app area,
  numbered step-by-step instructions for every happy path, quoting button/field/message
  labels exactly as rendered, plus a "Problemas comunes" table of real error messages.
  Any slice that adds or changes a user-visible flow updates its guide page in the same
  commit — a slice isn't done until its page is. Plain markdown, so it can be mirrored to
  a GitHub wiki verbatim if that's ever wanted.
- **UI/UX quality.** Tracked in [UX_QUALITY.md](UX_QUALITY.md) in three tiers: compliance
  (HTML semantics, WAI-ARIA, WCAG 2.2 AA; a failure is a bug), quality (Nielsen
  heuristics, Core Web Vitals, task metrics) and visual design (a five-colour palette and
  type scale in `static/app.css`, one primary action per screen, stable layout). It holds
  the per-screen scorecard and the issue log.

## Quote persistence

Every generated quote persists permanently. The spreadsheet has no history, so nobody can
answer "what exactly did we send Grupo PEME in March, and at what copper price?"

1. **Issued quotes are immutable.** `quote_lines` snapshot `description`,
   `unit_price_micros`, and the inputs that produced it (`kg_per_m`, `margin`,
   `metal_price`, `fx_rate`) as JSON. Nothing reads live prices when reprinting.
2. **Revisions are new rows.** A changed quantity creates a new quote with
   `supersedes_quote_id` and folio suffix (`QA0105-R1`). The original is untouched.
3. **The PDF is regenerated, not stored.** Everything it prints is frozen on the row at
   issue time (lines, totals, terms, `valid_until`, and the customer and salesperson names
   as `customer_name_snapshot`/`vendedor_snapshot`), and Typst renders byte-identically
   once its creation timestamp is pinned to `issued_at`. So a reprint always carries
   exactly the numbers, names and terms the customer received. The layout is *not*
   frozen: a template or Typst upgrade restyles old quotes too, a deliberate trade
   (decided 2026-09-22) over storing files and keeping every old template alive.
   `pdf_sha256` records the hash as issued; `Quotes.PDF` logs a warning when a reprint
   no longer matches it.

**Tables**: `quotes` (`folio` unique, `prefix` QA/QS/QI, `customer_id`, `user_id`, `status`,
`currency`, `fx_rate_used`, `subtotal`, `iva`, `total`, `terms_snapshot`, `created_at`,
`issued_at`, `valid_until`, `supersedes_quote_id`, `customer_name_snapshot`,
`vendedor_snapshot`, `pdf_sha256`) and
`quote_lines` (`quote_id`, `line_no`, `product_id` **nullable** — the "Cotizador libre"
sheet proves free-text lines are real — `description_snapshot`, `qty`,
`unit_price_micros`, `line_total`, `pricing_inputs` JSON, `source`, `rfp_extraction_id`).

Drafts are mutable; setting `issued_at` freezes the row and records the PDF's hash.

## Observability and auditing

Two separate questions, with two separate answers. The premise that they share one —
"turn on the binlog" — does not survive contact with SQLite, which has no binlog and no
logical replication: the WAL is a physical page log, so the CDC tooling built for
MySQL/Postgres (Debezium, `wal2json`, Maxwell, `pgaudit`) has nothing to attach to and
all of it needs a database *server* besides.

**Request logs: stdlib.** `log/slog` writing JSON to stdout, plus an access-log
middleware in `internal/web/logging.go` that also converts a handler panic into a 500
instead of a dropped connection. The one dependency is `github.com/felixge/httpsnoop`
(MIT, no transitive deps) for capturing the status code and byte count — the naive
`ResponseWriter` wrapper everyone writes by hand silently breaks `http.Flusher` and
`http.Hijacker`. Retention is the `json-file` driver's `max-size`/`max-file` in
`compose.yaml`, and `docker logs cladex` is the query interface. A shipper (Loki,
Vector, OpenTelemetry) is rejected on the same grounds as Chromium in M0: it would cost
more memory on the shared 8 GB box than the app it watches.

Metadata only, never bodies — `POST /login` carries a plaintext password and
`POST /clientes` carries RFC and contact details. Query strings are dropped too, since
`?q=` on the list pages is customer search text. Static assets and `/healthz` log at
Debug so the steady background traffic doesn't bury everything else; a *failing* static
request still logs at Warn.

**Row auditing: SQLite triggers**, in `migrations/0004_audit_log.sql`. Every insert,
update, and delete on `products`, `price_breaks`, `customers`, `users`, `settings`,
`quotes`, `units`, and `product_unit_conversions` writes an `audit_log` row holding the
audited columns before and after as JSON. Conversions are in for the same reason prices
are: a wrong "1 rollo = 100 m" silently multiplies a quote line.
Triggers rather than application-level logging because they fire for *every* writer of
the file — the web app, `cladex user ...`, `cmd/import`, and anyone who opens the DB with
the sqlite3 shell on the NAS. A store method added next year that forgets to log is still
audited.

What triggers cannot see is *who*: SQLite has no session user. `store.WithActor` puts the
actor on the context (`RequireAuth` attaches the session user, `cmd/cladexctl` and
`cmd/import` attach their source), and `store.exec` stamps the single-row `audit_actor`
table inside the same transaction as the write, which the triggers read. It stamps on
every write including unattributed ones, so a CLI edit is never misattributed to whoever
happened to write last. Two constraints found the hard way: a trigger cannot reference a
TEMP table (`cannot reference objects in database temp`), which rules out the usual
per-connection scratch-table trick; and the stamp must share the write's transaction, so
a failed write leaves no orphan audit row.

`users.password_hash` is never written to the trail — a bcrypt hash is a credential, and
the audit log is the most-read table during an incident. A `password_changed` boolean
records the fact instead. `audit_log` is append-only, guarded by `RAISE(ABORT)` triggers
on UPDATE and DELETE; a future retention policy has to drop them in a reviewed migration,
which is the point. `quote_lines` is deliberately not audited: it is rewritten on every
draft edit, and an issued quote already snapshots everything that matters.

At five users this table grows by a few thousand rows a year, so there is no pruning
story and doesn't need to be one. `store.AuditLog` reads it back with a
table/row/actor filter; no UI yet.

**Point-in-time recovery: Litestream** (`litestream.yml`, a sidecar service in
`compose.yaml`). This is the honest analogue of a binlog for SQLite — it ships WAL frames
continuously and restores to any second. It answers "what did the database look like on
Tuesday", which the audit trail does not, just as the trail answers "who changed this",
which Litestream does not. `backup.sh` keeps its job: it covers the quote PDFs, which are
files and invisible to Litestream, and produces a tarball that restores with no tooling.
What Litestream closes is the up-to-24h window in which a lost disk cost a day of quotes.
The replica is a local file under `$BACKUP_DIR`, which the NAS cloud sync already carries
offsite — so no cloud credentials live on the box. Note that Litestream v0.5 dropped age
encryption, so that replica is plaintext.

---

# Session-sized slices

- **Every slice ends deployable.** `main` is never broken.
- **Every slice has one verification command** that either passes or doesn't.
- **Owner is explicit.** Infra slices are agent-executable only after 0.0b establishes
  passwordless SSH; until then they are runbooks the user executes.

Legend: **[me]** = implementation session · **[you]** = runbook the user executes

## M0 — Walking skeleton and pipeline — ✅ COMPLETE

| # | Slice | Owner | Done when | Status |
|---|---|---|---|---|
| 0.0a | Local toolchain: Go, `brew install typst`, container runtime (**OrbStack** recommended; colima if FOSS preferred) with Rosetta for x86_64 | you | `docker run --rm --platform linux/amd64 alpine uname -m` prints `x86_64` | ✅ |
| 0.0b | SSH: generate a key, install it on the NAS, add a `Host cladex-nas` entry to `~/.ssh/config`, confirm docker-group membership | you | `ssh cladex-nas docker ps` succeeds **with no prompt** | ✅ |
| 0.1 | `go.mod`, `cmd/server`, ServeMux, templ layout + `/`, `static/` with htmx, `/healthz` returning build SHA | me | `go run ./cmd/server`; `/healthz` 200, `/` renders, htmx swap works | ✅ |
| 0.2 | `internal/money` + **full schema** in `migrations/0001_init.sql` via `embed.FS`; `/` renders `count(*) FROM quotes` | me | Migration applies clean; money round-trip tests pass; `/` shows `0 cotizaciones` | ✅ |
| 0.3 | `internal/pdf/typst.go`; `/sample.pdf` renders hello-world | me | `curl -o t.pdf localhost:8090/sample.pdf` opens as a valid PDF | ✅ |
| 0.4 | `Dockerfile` (3-stage, distroless, **linux/amd64**) + `compose.dev.yaml` | me | amd64 image builds and runs locally; all three endpoints answer from inside it | ✅ |
| 0.5 | `.github/workflows/deploy.yml` → GHCR, tagged `latest` + `sha` | me | Actions green; `ghcr.io/gerrygoo/cladex-web:latest` public and pullable | ✅ |
| 0.6 | NAS: `/volume1/docker/cladex/data`, `compose.yaml`, first manual `pull && up -d` | me¹ | `ssh cladex-nas curl -s localhost:8090/healthz` returns the pushed SHA; `docker stats` < 100 MB | ✅ |
| 0.7 | VPS + DNS: FRP entry, two nginx blocks, A + CNAME, certbot for both names | you²→me | `https://cotizador.cladex.com.mx/healthz` works; `c.` 301s to it | ✅ |
| 0.8 | Auto-deploy cron (5 min) + end-to-end push test | me¹ | Change text on `/`, push, ≤5 min later it's live, nothing manual | ✅ |

¹ Agent-owned **once 0.0b passes**. Installing the cron in 0.8 needed `sudo`, which
prompted for a password and returned to the user, same as anticipated.
² VPS access was not established from this machine at plan-writing time, but got set up
mid-M0 (Hetzner console → `authorized_keys`, 1Password SSH agent, `cladex-vps` alias) —
see the Environment section above. 0.7 ended up agent-executable after all, apart from
one classifier-gated step (certbot's `--agree-tos`, confirmed with the user).

`internal/money` lands in 0.2, before any schema, because the fixed-point representation
determines the column types.

## M1 — Auth and CRUD (the MVP)

| # | Slice | Owner | Done when |
|---|---|---|---|
| 1.1 | Backups: nightly `sqlite3 .backup` + the `quotes/` PDF dir, offsite copy | me¹ | Restore into a scratch dir and open it — **before 1.2 loads real data** — ✅ |
| 1.2 | `cmd/import` — parse the 4 catalog sheets into families, products, price_breaks | me² | Idempotent; row counts match; spot-check 5 SKUs by hand — ✅ (3 of 4 families; ELECTRACLEAN deferred) |
| 1.3 | Auth: bcrypt, sessions table, login/logout, middleware, rate limiting, CSRF | me³ | Log in, hit a protected route, log out → 302. 6th bad password is throttled — ✅ |
| 1.4 | `cladexctl user add/passwd/disable` + change-own-password page | me⁴ | Create a user via CLI, log in as them, change the password, log in again — ✅ |
| 1.5 | Products CRUD — list, search, create, edit, soft-delete | me | Full lifecycle through the UI; persists; works with JS disabled — ✅ |
| 1.6 | Customers CRUD | me | Same — ✅⁷ |
| 1.7 | Users admin + settings (FX, metal prices, margins), admin-gated | me | Vendedor gets 403 on both; admin can edit FX and see it reflected — ✅ |

⁵ `internal/store/products.go` gained the CRUD half (`ListFamilies`, `ListProducts`
with case-insensitive substring search — LIKE wildcards in the query text are
escaped so a literal `%` or `_` in a SKU/description doesn't act as one —,
`ProductByID`, `ProductBySKU`, `CreateProduct`, `UpdateProduct`,
`SoftDeleteProduct`); the import-only upsert half was already there from 1.2.
Soft-delete uses `products.deleted_at`, mirroring `customers` (1.6 will follow the
same pattern) — the pre-existing `products.active` column is untouched by this
slice, left for a possible future distinction between "soft-deleted" and
"discontinued but kept for quote history". `internal/money` gained `ParseMicros`
(decimal string → `Micros`, never through `float64`, so a catalog value's full six
digits of precision round-trips through a form field exactly) and `Micros.String()`
(the inverse, trailing zeros trimmed, for prefilling the edit form). Routes:
`GET /productos` (list + search), `GET/POST /productos/nuevo`,
`GET/POST /productos/{id}`, `POST /productos/{id}/eliminar` — all `RequireAuth`
only, no admin gate, since vendedor already has full product read/write per the
Auth design section. Search is a live htmx swap of just `#productos-tbody`
(`hx-trigger="keyup changed delay:400ms, search"`) with a plain `GET` form
fallback for JS-disabled — verified both paths actually work in a real browser,
not just via the httprecorder tests (an earlier pass looked broken because a
`templ generate` was missed after a late edit to `layout.templ`; the dev server
was serving a stale nav bar with no "Productos" link — a reminder to always
regenerate and restart before trusting a browser check). Create/update use
standard POST + redirect (to the edit page with `?guardado=1`, not back to the
list) rather than mi-cuenta's render-in-place, since that's the plan's stated
default and lets you immediately see the saved values. Duplicate SKU is checked
with a pre-`INSERT` `ProductBySKU` lookup rather than parsing the driver's UNIQUE
violation error, for a friendly field error instead of a 500 — matches the
codebase's existing preference for plain `database/sql`, no driver-specific error
introspection anywhere else either. Verified end-to-end in a real browser against a
fresh dev DB (two seeded families, no import): create with a six-decimal unit
price → list shows it rounded to centavos → edit page shows the un-rounded
`6.319872` prefilled → live search filters correctly and updates the URL via
`hx-push-url` → soft-delete removes it from the list. Also covered by
`go test ./...` (store CRUD + search escaping, web handlers for create/validate/
duplicate-SKU/update/soft-delete/htmx-fragment-response/auth-required).

⁶ Scope call, not fully forced by the plan text: `/usuarios` manages role and
enabled/disabled state for *existing* accounts only — no create-user, no
password-reset-via-web. The Auth design section is explicit that provisioning is
out-of-band with no self-service surface ("no signup, no public password reset");
extending that to a web page that could mint a fresh password or a brand-new
account would cut against that decision, so creation and password resets stay
`cladexctl`-only (1.4). `internal/store/users.go` gained `ListUsers`,
`SetUserDisabled` (by id, unlike the CLI's `DisableUser` which takes a username),
and `SetUserRole`. A new `internal/store/settings.go` wraps the already-existing
generic `settings` key/value table with `SettingValue`/`SettingValues`/
`SetSetting`; `/ajustes` exposes three fixed knobs (`fx_rate`, `copper_price`,
`default_margin`) as a hardcoded list in `internal/web/ajustes.go` rather than
walking the table generically, since the UI needs a Spanish label and a
placeholder per field — adding a fourth knob later (e.g. an aluminum price, once
the deferred ALUMOCLAD family lands) is a one-line addition to that list, no
migration, per the table's original design intent. Settings values reuse
`internal/money.ParseMicros`/`Micros.String()` from 1.5 — same fixed-point
round-trip, so `0.35` for a 35% margin persists and redisplays as `0.35`, not
`0.350000` or a float-drifted approximation. `/ajustes` submission is all-or-nothing:
if any of the three fields fails to parse, nothing is written, and all three
fields redisplay with the entered (possibly invalid) values per the plan's form
convention. Both `/usuarios` and `/ajustes` are gated
`auth.RequireAuth(auth.RequireAdmin(handler))` — the first real exercise of
`RequireAdmin` against a live route rather than only in `auth_test.go` (built in
1.3). Guarded against self-lockout: an admin can't change their own role or
disable their own account through `/usuarios` — the UI hides those controls on
the signed-in admin's own row, and the handler re-checks server-side so a crafted
POST can't bypass it either. `NavUser` gained an `IsAdmin` field so the layout can
show/hide the "Usuarios"/"Ajustes" nav links; `navUserView` sets it from
`AuthenticatedUser.Role`. Verified end-to-end in a real browser: logged in as a
plain vendedor, confirmed both nav links are absent and direct navigation to
either route 403s ("prohibido"); logged in as admin, changed a user's role,
disabled and re-enabled a different user, saved all three settings, then reloaded
`/ajustes` from a fresh request (not just the POST response) to confirm the
values actually round-tripped through SQLite. Also covered by `go test ./...`
(settings round-trip, `ListUsers`/`SetUserDisabled`/`SetUserRole`, and web
handlers for both routes including the 403-for-vendedor and self-lockout cases).

⁷ Built after 1.7 (user request), mirroring 1.5's shape exactly — same
list/search/create/edit/soft-delete pattern, same file layout
(`internal/store/customers.go`, `internal/web/customers.go`,
`internal/views/customers.templ`). `customers` has no equivalent of products'
`family_id`/pricing fields, so the form is simpler: `name` required, everything
else (`rfc`, `contact_name`, `phone`, `email`, `address`, `notes`) optional free
text, stored as SQL `NULL` when blank via a small `nullIfEmpty` helper rather
than empty strings — kept the intent ("not provided" vs "explicitly blank")
visible in the schema, matching how the column was already nullable. Search
matches name, RFC, or contact name (`ListCustomers`), reusing the same
`escapeLike` wildcard-escaping helper `products.go` already defined in the
`store` package. Routes are `RequireAuth`-only, no admin gate, same reasoning as
products (vendedor already has full customer read/write per the Auth design
section). Verified end-to-end in a real browser as a vendedor: create a
customer with RFC/contact/phone/email → edit page shows it all prefilled → list
row shows it → live search matches on contact name, not just company name →
plain-GET fallback (typed a query, pressed Enter — no live-search JS path)
returns the same filtered result → soft-delete removes it from the list. Also
covered by `go test ./...` (store CRUD + search-across-three-columns, web
handlers for create/validate/update/soft-delete/htmx-fragment-response/
auth-required).

**1.2 is the riskiest slice.** The sheets are irregular — merged headers, a hidden
`Descripción | Precio` block on the right of each, per-family layouts that disagree.

³ `internal/store/{users,sessions,login_attempts}.go` + `internal/web/auth.go`.
Sessions: SHA-256(token) in `sessions.token_hash`, sliding 14-day expiry (renewed at
most once/day per session, computed in SQL — no Go-side time parsing needed since
timestamps stay in the DB's own ISO-8601 string format). Cookie is `HttpOnly`,
`SameSite=Lax`, and `Secure` by default; `COOKIE_SECURE=false` (set in
`compose.dev.yaml`) opts out for local plain-HTTP dev, since the app itself never
terminates TLS (nginx/VPS does) so `r.TLS` is always nil regardless of environment.
CSRF via stdlib `http.CrossOriginProtection` wrapping the whole mux — zero deps, and
requests without `Origin`/`Sec-Fetch-Site` (curl, non-browser) pass through untouched.
Rate limiting counts failed `login_attempts` since each username/IP's last success
(so a successful login resets the streak), exponential backoff `2^(failures-5)`
seconds capped at 5 minutes, checked *before* the password comparison so the 6th
attempt is rejected without even touching bcrypt. Login also runs bcrypt against a
fixed dummy hash when the username doesn't exist, so a failed login takes the same
time either way — defeats timing-based username enumeration. `RequireAuth` /
`RequireAdmin` middleware are built and unit-tested (403 for vendedor, 200 for
admin) — there are no real admin routes to gate yet (`/usuarios`, `/ajustes` are
1.7), so RBAC enforcement is proven in `internal/web/auth_test.go` rather than
against a live route. Verified end-to-end against a locally seeded dev DB (two
throwaway users, one per role, created directly via `store.CreateUser` — not through
`cladexctl`, which is still 1.4): login sets the cookie, `/` requires it and shows
the logged-in username, logout clears the session, and the 6th bad password gets a
429 with `Retry-After`. `golang.org/x/crypto` (bcrypt) promoted from an indirect to
a direct dependency; no schema changes — `sessions` and `login_attempts` were
already in `migrations/0001_init.sql`.

⁴ Resolved a real ambiguity between this plan's "Application structure" (a separate
`cmd/cladexctl` binary) and its own worked provisioning example (`docker compose exec
cladex /cladex user add ...`, reusing the server's binary): the deployed image ships
only `/cladex` (see `Dockerfile`'s `COPY --from=build /out/cladex /cladex`), so
`cmd/server/main.go` now dispatches `user add/passwd/disable` itself when
`os.Args[1] == "user"`, sharing the actual logic with the rest of the app via a new
`internal/cli` package. `cmd/cladexctl` was also built as a thin wrapper around the
same package, purely for local-dev convenience (`go run ./cmd/cladexctl user add
...` without the server's env-var assumptions) — it isn't part of the deploy image
or the documented provisioning command. Caught by testing the plan's *exact* worked
command during verification: Go's `flag.Parse` stops at the first non-flag token, so
`cladex user add rodolfo --name ... --role ...` (username first, flags after) failed
until the username was peeled off by hand before handing the rest to a `FlagSet` —
would have shipped broken if only tested as `add --name ... --role ... rodolfo`.
`user add` generates a 16-char random password (ambiguous characters like `0`/`O`
and `1`/`l`/`I` excluded, since an admin reads it once off a terminal), bcrypt cost
12, printed once — never stored in the clear. `user passwd` additionally deletes all
of that user's sessions (`store.DeleteSessionsByUserID`), since a reset is the
account-recovery path and an old stolen cookie shouldn't survive it; self-service
`/mi-cuenta` does not do this — it's not a recovery flow, and force-logging the user
out of their own change would be a bad experience for no security benefit. `user
disable` relies on the `SessionUser` query's existing `disabled_at IS NULL` check
for immediate effect — no separate session deletion needed. `/mi-cuenta` follows the
plan's `Validate() map[string]string` form convention (`ChangePasswordForm` in
`internal/web/mi_cuenta.go`) but deliberately does not echo password values back
into the re-rendered form on a validation error, unlike the general "preserve
values" rule — echoing password input back into HTML, even same-response, isn't
worth the marginal convenience. Verified end-to-end against a fresh local dev DB
using the actual `cladex user add` dispatch (not a direct `store.CreateUser` call,
unlike 1.3's verification): created a user, logged in, changed the password via
`/mi-cuenta`, confirmed the old password now fails and the new one works, and
confirmed `user disable` immediately blocks login.

¹ Deployed and verified: `backup.sh` runs nightly `sqlite3 .backup` + integrity check +
`quotes/` dir into a 14-day-retention tarball under `/volume1/docker/cladex/backups/`,
restore-tested into a scratch dir on the NAS. Offsite copy deferred — the user plans to
point UGREEN's own NAS-to-cloud sync at that directory once it's set up, so no bespoke
offsite push was built. Installing the nightly cron entry needs `sudo` on the NAS (same
wall as 0.8's deploy cron) — bounced to the user. Also learned: `scp` to the NAS fails
under the modern SFTP-based protocol (`dest open ... No such file or directory`); use
`scp -O` (legacy protocol) instead.

² Built and verified against the real workbook (`~/Downloads/Cladex catalogo precios.xlsx`
on the user's Mac — never committed, `-xlsx` flag points at it). Key discovery: every
catalog sheet has a *hidden* two-to-four-column mirror block elsewhere in the sheet that
the existing "Cotizador" quote forms actually `VLOOKUP` against — traced the real
formulas to confirm. That hidden block is Cladex's own clean, flattened
`(Descripción, Precio, ...)` list, immune to the merged-header/multi-category mess in the
raw columns, so the importer reads from there instead of trying to parse the raw layout
directly.

Per-family decisions (confirmed with the user, not assumed):
- **ABASTILUM**: flat catalog prices → `unit_price_micros`. No real SKU exists (the
  business VLOOKUPs on description alone), so SKU is `abl-<slug of description>`.
- **CCA** and **CCS & AC**: cost + margin families — margin is applied at quote time
  (M2), so the importer stores the pre-margin *cost*, not the mirror's margin-adjusted
  price. This needed a schema addition, `migrations/0002_add_product_cost.sql`
  (`products.cost_micros`, nullable — mutually exclusive with `unit_price_micros` in
  practice). SKU is `<family>-c<gauge>` (`cca-c14`, `ccad-c14` for the bare/DESNUDO
  variant) — except CCS & AC, where the raw AWG code collides across two products
  (`7#6 LC DSA` and `19#9 LC DSA` are both `3/0`), so its SKU is `ccs-<slug of name>`
  instead.
- **ELECTRACLEAN**: skipped entirely for now, per the user ("ignore everything related
  to electraclean") — no family row, no products. `price_breaks` (quantity-tiered
  pricing) is therefore still unused; ELECTRACLEAN was its only real use case in the
  current data.
- One outlier, "CABLE ALUMOCLAD CALIBRE 7#8", lives outside any mirror block (a
  hardcoded cell reference in the "Cotizador Alumoclad" form) — skipped, to be added by
  hand once 1.5 (Products CRUD) exists.

Verified: dry-run (`-dry-run`) prints every proposed row before any write; the importer
checks for cross-family SKU collisions and refuses to import if any are found; a real run
against a local dev DB produced 69 products across 3 families, re-running was a no-op row
count (idempotent via `ON CONFLICT (sku) DO UPDATE`), and 5 spot-checked rows matched the
source workbook exactly (in micros).

**Run against production** (with the user's explicit confirmation — writes to the live DB
aren't auto-approved): cross-compiled `cmd/import` for the NAS's amd64, staged it and the
workbook in a scratch dir there, dry-ran against the real production path first (matched
the local test exactly), then ran for real. Same 69/3 result, same spot-check. Scratch
dir (binary + workbook) deleted immediately after — the workbook never lives on the NAS
longer than the run itself. ELECTRACLEAN and the Alumoclad outlier are deferred to when
Products CRUD (1.5) exists, per the user.

## M2 — Quoting engine and PDF

| # | Slice | Owner | Done when |
|---|---|---|---|
| 2.1 | Pricing engine, pure Go, no HTTP: `kg/m × metal $/kg × (1+margin)`, USD×FX, qty breaks, IVA | me | Unit tests reproduce ≥15 known prices from the spreadsheet exactly — ✅⁸ |
| 2.2 | Quote builder UI — htmx shell + vanilla-JS island for live line editing and totals | me | Build a 5-line quote end to end; totals match 2.1 — ✅⁹ |
| 2.3 | Typst cotización template + render pipeline; per-family terms blocks | me | Generated PDF matches the current Excel output for the same input — ✅¹⁰ |
| 2.4 | Folio sequences, issue flow (freeze row, write PDF + SHA), status, revisions | me | Issue, change FX and copper, reprint → byte-identical PDF. Revise → `-R1`, original untouched — ✅¹¹ |
| 2.5 | Quote history: list, filter by customer/vendedor/status, re-download its PDF | me | Every quote ever issued is findable and re-downloadable |

**2.1 before 2.2, without exception.** The engine is testable against the spreadsheet's own
numbers; the UI is not. Prove the engine first and later UI bugs can never be mistaken for
pricing bugs.

⁸ `internal/pricing` (new package) + `internal/money` gained `RoundHalfUp` (the existing
micros→centavos rounding, generalized to an arbitrary denominator so margin and
line-total math share the same rule instead of duplicating it), `Milli` (1e-3 fixed-point
quantity type, for `qty_milli`/`min_qty_milli`), `LineTotalCentavos`, and `ApplyRate`.
Reverse-engineered the actual formulas from the workbook (`CCA`, `CCS & AC`, their
`Cotizador` forms, and the hidden `TIPO DE CAMBIO` settings block down in row 1048561+)
since there's no written spec — dumped every relevant cell's raw formula with a scratch
`excelize` program rather than guessing from the visible numbers. Three surprises that
shaped the design:
- The plan's shorthand `kg/m × metal $/kg × (1+margin)` doesn't match CCA's actual
  formula, which is `cost / (1 - margin)` — margin-on-*sale-price*, not markup-on-cost.
  Using `(1+margin)` with the same nominal fraction produces a different number, so the
  engine matches the workbook's convention exactly rather than the plan's paraphrase.
- CCS & AC's margin is *derived* from copper price in the sheet
  (`(copper_price - base_cost) / copper_price`), and algebraically cancels: feeding it
  back through `cost / (1 - margin)` reduces to exactly `kg_per_m × copper_price`, with
  no separate margin term at all. Confirmed by hand (not just numerically) before relying
  on it, since it means CCS & AC has no independent margin knob — the `Cotizador CCS`
  form's own line prices confirm there's no additional markup layered on top.
- Only CCA's margin needed a settable knob, and it's the one number in the whole workbook
  that isn't copper-price-derived or already baked into a flat price — so it maps
  directly onto 1.7's existing `default_margin` setting; no new settings knob or
  migration was needed. ABASTILUM's flat `unit_price_micros` (already FX- and
  margin-adjusted at 1.2's import time) needs no live formula at all, so the engine
  returns it unchanged; USD→FX conversion and price-break tier selection are implemented
  generically per the plan but have no real spreadsheet-sourced test data yet (no
  imported product is USD-priced or qty-tiered) — covered by synthetic tests instead.
Verified against 27 real prices pulled programmatically from the workbook (11 THW-2-LS +
7 CABLE DESNUDO rows from `CCA`'s hidden mirror block, 9 rows from `CCS & AC`'s), not
hand-transcribed — `internal/pricing/pricing_test.go`. `go test ./...` green.

⁹ Two decisions confirmed with the user before building, both diverging from this plan's
own shorthand: (1) a quote's `prefix` (QA/QS/QI) is a free choice made once at creation —
purely a folio-series label, matching the legacy workbook's four separate "Cotizador"
forms — and does **not** restrict which products the quote can contain; any product, any
family, plus free-text lines, on one quote. (2) the `quotes` row (and its folio) is
persisted the moment a draft is started, but `quote_lines` are **not** written per edit —
every add/remove/qty-change is a pure recompute (no DB write) via
`POST /cotizaciones/{folio}/recalcular`, and only the explicit "Guardar borrador" action
(`POST .../guardar`) replaces the whole line set in one transaction
(`Store.ReplaceQuoteLines`), so editing never produces a per-edit history row. Also
diverged from the plan's "vanilla-JS island" shorthand: the builder is pure htmx, no
custom JS at all — every line mutation needs a server round-trip anyway (unit price
depends on live FX/copper/margin settings), so there was no client-side math left to
speed up, and pure htmx keeps the codebase's existing "forms work without JS" rule intact
for free (every mutating control is a real `<form>` submit; `formaction` overrides pick
`recalcular` vs `guardar` per button, and a non-htmx request to `recalcular` gets the full
page back, not a bare fragment, so a no-JS user is never stranded).

Since `quotes.folio`/`prefix` are `NOT NULL` from the M0.2 schema, this slice had to build
*some* working folio-assignment scheme even though "Folio sequences" is 2.4's title — a
new `folio_sequences` table (`migrations/0005_add_folio_sequences.sql`) plus
`Store.NextFolio`, a single atomic `INSERT ... ON CONFLICT DO UPDATE ... RETURNING`
(confirmed `RETURNING` works against `modernc.org/sqlite`), giving `QA0001`-style folios
with no read-then-write race window. 2.4 still owns the revision suffix (`-R1`) and the
issue/freeze flow on top of this.

`internal/store/quotes.go` (new): `Quote`/`QuoteLine`, `NextFolio`, `CreateDraftQuote`,
`QuoteByFolio`/`QuoteByID`, `ListQuotes` (search+sort, same shape as
`ListProducts`/`ListCustomers`), `ListQuoteLines`, and `ReplaceQuoteLines` (the one
transactional delete+reinsert-lines-plus-update-totals write). `internal/store/products.go`
gained `ListPriceBreaks`, still exercising the `price_breaks` table for the first time
since 1.2 deferred its only real use case (ELECTRACLEAN) — wired in generically rather
than assuming an always-empty slice. `internal/web/quotes.go` (new) parses the builder
form's line set (`line_keys` + indexed `lines[K][...]` fields, `add_product_id`/
`add_free`/`remove_key` as the one delta per submit), prices every line through
`internal/pricing.UnitPrice`/`ComputeTotals` — the same code path for a live recalculation
and the final save, so nothing persisted ever differs from what was last shown — and
attaches a per-line error (bad qty, unknown product, missing FX/copper/margin settings)
without failing the whole request. `internal/views/quotes.templ` (new) follows the
existing list/form/htmx-fragment conventions (`ProductsList`/`ProductsTableBody`, etc.)
exactly. Routes are `RequireAuth`-only, no admin gate, same reasoning as products/customers.

Verified with `go test ./...` (folio-sequence increment-per-prefix, draft creation,
line-replace round-trip including totals, form validation, htmx-fragment vs full-page
responses, row-level pricing errors blocking a save, auth-required) and end-to-end in a
real browser: created a draft, added a flat-priced product and a cost+margin product from
two different families (proving no family restriction), added and priced a free-text
line, edited a quantity and watched the line and totals recompute live via htmx with no
page reload, removed a line, saved the draft, and confirmed from a fresh page load — and
directly in the SQLite file — that exactly the right rows persisted (no stale rows from
the removed line) with the correct `pricing_inputs` snapshot on the product line and
`NULL` on the free line. Also confirmed the no-JS fallback: the product search's plain
"Buscar" button (a real GET, no htmx) returns results embedded in the full page rather
than a bare fragment.

¹⁰ `internal/pdf/quote.go` (new): `RenderQuote`/`QuoteDocument` builds the Typst source
for a cotización — letterhead (logo + address), folio/cliente/fecha/vendedor header, a
line-item table, subtotal/IVA/total, and a `QuoteTerms` map keyed by folio prefix
(QA/QS/QI) holding each family's terms block, transcribed from the legacy workbook's
separate "Cotizador CCA"/"Cotizador CCS"/"Cotizador Alumbrado" sheets (each ends in its
own list — cable specs and packaging terms differ by family). Every dynamic value
(customer name, product/free-line descriptions — none of it under this app's control) is
bound via Typst `#let` to an escaped string literal and interpolated with `#name`, never
spliced into markup source directly: interpolating a `str` value inserts its literal
text without re-parsing it as markup or code, so content containing Typst
metacharacters (`#`, `*`, `[`, `$`, a literal backslash) can't break out of its cell or
execute as Typst code — confirmed by compiling adversarial input containing
`#read("...")` and confirming it renders as inert text (`internal/pdf/quote_test.go`).
The Cladex logo is embedded as a Typst `bytes()` literal generated from a `//go:embed`
PNG, not read from a file path, so `Render`'s pure stdin/stdout subprocess interface
(unchanged since 0.3) needed no `--root`/filesystem access. `Quotes.PDF`
(`GET /cotizaciones/{folio}/pdf`, `internal/web/quotes.go`) renders a draft's *current*
state through the same `computeQuoteLines` path the builder and Guardar use, so the PDF
always matches what's on screen — available as a live preview during drafting; freezing
and persisting the PDF bytes on issue is still 2.4's job. `store.Quote` gained `UserName`
(joined from `users`, alongside the existing `CustomerName` join) so the PDF can show a
vendedor name without a second query.

Verified against the real workbook (`~/Downloads/Cladex catalogo precios.xlsx`, same file
1.2 used): imported a fresh dev DB, set `fx_rate`/`copper_price`/`default_margin` to the
workbook's own `TIPO DE CAMBIO` sheet values (`$18.00`, `$220.00`/kg, and 0.1234 — the
*true* CCA margin is 12.34%, traced through `Cotizador CCA`'s `D19` formula chain to
`CCA!J4 = E4/'TIPO DE CAMBIO'!C1048561`, not the sheet's rounded-for-display "Margen CCA
(%) = 12%" label), then built one quote per prefix reproducing the exact line items from
each family's Cotizador sheet. Every one of the 18 CCA/CCAD line prices and all 9 CCS/AC
line prices matched the workbook exactly, to the centavo (e.g. THW-2-LS calibre 14 →
$6.32, Cable CCS 30% ALAMBRE 4 → $37.91), and each PDF's terms block matched its sheet's
list verbatim. One expected, pre-existing divergence: the workbook's own subtotal sums
*unrounded* line prices and rounds once at the end ($1,232.10 for the CCA example), while
this app's subtotal sums each line's already-rounded total (`$1,232.09`) per the money
design's "round exactly once, at line total" rule (`docs/PLAN.md`'s Money section,
decided at 0.2) — a one-centavo aggregate drift on large multi-line quotes, not a pricing
error on any individual line, and not something this slice changes.

This verification pass also caught a real, previously-shipped pricing bug, unrelated to
the PDF work itself: `cmd/import`'s `parseCCSAC` populated *both* `CostMicros` and
`KgPerMMicros` on every CCS & AC product, and `pricing.basePrice`'s field-presence
dispatch (`internal/pricing/pricing.go`) checks `CostMicros` before `KgPerMMicros` — so
every CCS/AC line has been silently priced with CCA's `cost / (1 - margin)` formula
instead of the correct `kg_per_m × copper_price` (footnote 8) ever since 2.2 shipped
`computeQuoteLines`. `internal/pricing/pricing_test.go`'s own CCS test cases never caught
this because they construct a synthetic `Product{KgPerMMicros: ...}` with `CostMicros`
left nil — the real bug only appears when a real imported product (which had both fields
set) flows through the actual store → web → pricing path, which no test exercised until
this manual PDF check. Fixed by no longer importing CCS & AC's "cost before margin"
column at all (`cmd/import/parse.go`): that family has no independent margin knob, so the
value was never meaningful pricing input, only a latent trap once stored alongside
`kg_per_m_micros`. CCA is unaffected — it legitimately stores both fields (cost as the
pricing input, kg/m as reference-only weight data) and `CostMicros` winning first is
correct for that family. Added `cmd/import/parse_test.go` (`TestParseCCSACOmitsCost`) as
a regression test, since `cmd/import` had no tests before this.

**Fixed in production**, not by re-running `cmd/import`: the user flagged that sellers
may have hand-edited some product rows since 1.2's import, and a full re-import
overwrites every column from the spreadsheet (not just the broken one), so it could
silently clobber those edits — confirmed this risk was real when the dry-run showed
production's CCS & AC SKUs use an older scheme than the current importer generates
(`CCS-A#4` vs. `ccs-alambre-4`), meaning a SKU-matched re-import might not even have
lined up with the existing rows correctly. Instead wrote a narrowly-scoped, one-off
spot-fix (`cmd/spotfix-ccs-cost`, not committed — deleted after use) that clears only
`products.cost_micros`, only on rows in the `CCS & AC` family (matched by
`product_families.name`, not SKU): `UPDATE products SET cost_micros = NULL WHERE id IN
(SELECT p.id FROM products p JOIN product_families f ON f.id = p.family_id WHERE
f.name = 'CCS & AC' AND p.cost_micros IS NOT NULL)`. Verified locally first against a
scratch DB seeded with a fake "user edit" (changed description + kg/m) on one of the
affected rows, confirming the fix left it untouched. Then, with the user's explicit
go-ahead, cross-compiled for the NAS's `amd64`, staged it via `scp -O`, dry-ran against
the real production DB (`/volume1/docker/cladex/data/cladex.db`) to show the exact 9
rows first, ran for real, verified a second dry-run found nothing left, confirmed
`/healthz` the whole time, and deleted the binary from the NAS.

¹¹ Extended `store.Quote` with the freeze/revision fields the M0.2 schema already
reserved (`FxRateUsedMicros`, `TermsSnapshot`, `IssuedAt`, `ValidUntil`,
`SupersedesQuoteID`, `PDFPath`, `PDFSHA256`), plus two read-only convenience fields
populated by correlated subqueries in `quoteSelectCols` — `SupersedesFolio` (the folio
this one revises, if it's a revision) and `SupersededByFolio` (the folio that
superseded this one, if any) — so the UI never needs a second query to link a
revision chain together.

**Issue flow** (`Store.IssueQuote`, `Quotes.Emitir` at `POST
/cotizaciones/{folio}/emitir`): the "Emitir cotización" button lives on the same
`<form id="linea-form">` as "Guardar borrador" (a `formaction` override, same pattern
as the rest of the builder), so issuing also saves whatever's currently on screen —
no separate "save first, then issue" step. `IssueQuote` is a single `UPDATE ... WHERE
id = ? AND status = 'borrador'`, so two concurrent issue attempts can't both succeed
and a second `Emitir` on an already-issued quote gets `ErrQuoteNotDraft` (surfaced as
409). Freezing captures: the FX rate actually in `Ajustes` at that moment
(`fx_rate_used_micros` — always recorded, not only for USD lines, since it's cheap,
useful audit context either way); the family's terms text, newline-joined into
`terms_snapshot` — critical for "reprint → byte-identical", since without freezing it
a future edit to `pdf.QuoteTerms` would silently change what an old issued quote
reprints as; a `valid_until` 30 days out (`defaultValidityDays` — no UI exists yet to
pick a custom validity per quote, a scope call, not a schema limitation); and the
rendered PDF's bytes, written to `<dataDir>/quotes/<folio>.pdf` and SHA-256'd
(`pdf_path`/`pdf_sha256`). `dataDir` threads from `cmd/server/main.go`
(`filepath.Dir(dbPath())`, so it's `data/` locally and `/data` in the container,
alongside `cladex.db` — no new env var) through `web.NewMux` into `NewQuotes`.

**Reprint** (`Quotes.PDF`): branches on `quote.PDFPath` — set, it serves those exact
bytes straight off disk (`os.ReadFile`, joined against `quotesDir` via
`filepath.Base` so a corrupted/adversarial `pdf_path` value can't path-traverse);
unset (still a draft), it live-renders a preview through the same `computeQuoteLines`
path the builder uses, using the *live* `pdf.QuoteTerms` (a draft has no snapshot
yet to freeze from — see `resolveTerms`). Verified the byte-identical guarantee two
ways: a `go test` that renders, then changes `fx_rate`/`copper_price` in the DB, then
re-fetches and diffs the response bodies; and manually in a real browser — issued a
quote, downloaded the PDF, changed FX/copper/margin to wildly different values via
`/ajustes`, downloaded again, and confirmed the SHA-256 was identical both times.

*Follow-up fix (found while writing `docs/guia/`):* the PDF half of immutability held,
but the **on-screen** half didn't — `Quotes.Builder` ran every quote, issued or not,
through `computeQuoteLines`, so an issued quote's page showed live-repriced lines (a
margin change moved a frozen $64.29 line to $90.00) and silently dropped any line whose
product had since been soft-deleted. Builder now recomputes only drafts; past
`'borrador'` it renders `frozenQuoteLines` (the stored `description_snapshot`,
`qty_milli`, `unit_price_micros`, `line_total`) with the stored
`subtotal`/`iva`/`total`. `Recalcular` also 409s on non-drafts, matching
`Guardar`/`Emitir`. Guarded by `TestQuotesIssuedPageShowsFrozenLines`.

**Revisions** (`Store.CreateRevision`, `Quotes.Revisar` at `POST
/cotizaciones/{folio}/revisar`): only callable on a quote currently `'emitida'`
(`ErrQuoteNotIssued` otherwise, 409) — the active version of a lineage, never a
`'revisada'` one, so revising has to go through whichever quote is currently active.
One transaction: compute the next folio (`baseFolio` strips any existing `-R<n>`
suffix via regex, so revising `QA0105-R1` produces `QA0105-R2`, not
`QA0105-R1-R1`; the next number is the highest existing `<base>-R%` folio + 1, found
with a plain `LIKE` — safe without escaping because every folio is machine-generated,
never user-typed), insert the new `'borrador'` row (`supersedes_quote_id` pointing at
the quote just revised, customer/prefix/currency copied across), copy the original's
`quote_lines` verbatim as a starting point (not re-priced — reopening the revision in
the builder recomputes them live from current settings exactly like any other draft's
persisted lines would), copy its totals so the new draft displays correctly before any
edit, then flip the original's `status` to `'revisada'` — the *only* write to the
original's row; its folio, lines, totals, `terms_snapshot`, and PDF are never touched
again. `Guardar` also rejects a non-`'borrador'` quote (409) as a second layer of
immutability enforcement, independent of the UI not rendering edit controls for one.

Verified with `go test ./...` (issue freezes exactly the expected fields and rejects a
second issue; issuing with an invalid line saves nothing and leaves the quote a draft;
a revision copies lines/totals, marks the original `revisada`, and a further revision
of the *original* is rejected while revising the *new* revision correctly chains to
`-R2`; the byte-identical-reprint-after-settings-change check above) and end-to-end in
a real browser: issued a quote (5 units), confirmed the read-only view, frozen FX/
vigencia, and stored PDF file; clicked "Revisar", landed on an editable `-R1` draft
pre-filled with the same line and linking back to the original; changed the quantity
to 8, watched totals recompute live, and issued the revision; reloaded the *original*
and confirmed it now reads "revisada" with its original qty-5/$36.66 totals completely
unchanged and a link forward to `-R1` — proving the original truly never moved once a
newer revision existed.

*Superseded 2026-09-22 — PDFs are regenerated, not stored* (migration
`0006_regenerate_quote_pdfs.sql`). The stored-file design above had a hole: the PDF's
customer and salesperson names came from live joins, so they were frozen only by
accident of the file existing. And the files needed their own backup path outside
Litestream. Now `Emitir` freezes those names into `customer_name_snapshot`/
`vendedor_snapshot`, picks `issued_at` in Go (whole seconds), renders once through
`issuedQuoteDocument` to record `pdf_sha256`, and writes nothing to disk; `Quotes.PDF`
re-renders issued quotes through the same function, with `pdf.Render` passing
`--creation-timestamp` = `issued_at` so the output is byte-identical. `pdf_path`,
`quotesDir` and the `dataDir` plumbing are gone; the migration backfilled snapshots for
the (mock) quotes already issued and cleared their old, unreproducible hashes. Guarded
by `TestRenderQuoteIsDeterministic`, `TestQuotesEmitir` (reprint matches the issue-time
hash after settings, product price and customer name change) and
`TestMigration0006BackfillsNameSnapshots`.

## M3 — Standardized margins and a materials catalog — ✅ COMPLETE

Brainstormed and settled with the user on 2026-09-22; shipped 3.1–3.3 by 2026-09-28 (3.4 folded into 3.2). Follow-up done 2026-09-28: the QI terms block now says "Precios en pesos mexicanos (MXN)" like QS/QA, and the "$93" percha line is gone (user's call). Quotes issued before that keep their frozen terms.

### Why: margin is hidden inside costs and the FX rate today

Every family *does* have a margin, but only CCA's is an explicit input. The others are
baked into a cost, the FX rate, or a flat price. Traced from the workbook's own formulas
(`Cladex catalogo precios.xlsx`):

| Family | Workbook formula | Where the margin actually lives today |
|---|---|---|
| CCA | `cost / (1 − 12.34%)` (`CCA!J4`) | The `default_margin` setting (explicit). The sheet also shows three price columns, "Cladex 12.34% / 16.56% / 20.28%": a margin menu the workbook already has, hardcoded. |
| CCS & AC | `kg/m × $155/kg / (1 − "Margen CCS")`, where Margen CCS = `(220 − 155) / 220` | Inside the `copper_price` setting. The 220 is a *sale* price per kg; the real material cost is $155/kg (`CCS & AC!D4`). The ≈29.55% margin cancels out algebraically (footnote 8), so it's invisible. |
| ABASTILUM, postes | `cost / (1 − 20%) × FX 18` | Baked into the imported flat `unit_price_micros`, along with the FX rate. The sheet's own header asks "USD?". |
| ABASTILUM, LEDVANCE | `cost / (1 − 12.34%)` | Baked into the flat price. It reuses **CCA's** margin cell, which is almost certainly an accident. |
| ABASTILUM, PHILIPS and the rest | `cost / (1 − 20%)`, MXN, no FX | Baked into the flat price ("Margen iluminación"). |

`TIPO DE CAMBIO` also holds unused "Margen electraclean 20%" and "Margen Fire Blanket
25%" cells.

### Decisions (user, 2026-09-22)

1. **One pricing formula for every product:** `price = cost / (1 − margin)`, the same
   margin-on-sale-price convention CCA already uses. Margin is never again part of a
   cost, a material price, or a flat price.
2. **Margins are a menu, an N-tuple of named options.** At any moment the source of
   truth is the set of options currently defined. There is **one global menu** (not per
   family), and it is admin-maintained.
3. **The margin is chosen per quote**, not per line. Vendedores can pick any option;
   they cannot edit options, materials, or any other setting.
4. **Drafts follow.** A draft references an option, not a frozen value, so editing an
   option reprices every open draft that uses it. The number freezes only at `Emitir`.
5. **No currencies and no FX at all.** Every cost is MXN. That drops `products.currency`,
   `quotes.currency`, `quotes.fx_rate_used_micros`, the USD branch in `internal/pricing`,
   and the `fx_rate` setting. Production needs none of it: all 69 products are MXN and
   there are no USD lines on any quote (checked 2026-09-22 against the nightly backup).
   If a supplier bills in USD, the admin enters the MXN cost. YAGNI: support can be
   added back if a real USD product ever appears.
6. **N materials, each with an MXN price per unit** (e.g. CCS 30% at $160/kg; later
   copper, aluminum, ...). Material prices are pure cost, with no margin. CCS 30% is
   **$160/kg** (user, 2026-09-22: production's copper_price of $160 is the material
   cost and unlikely to change this year), not the workbook's $155.
7. **Conversions are unit conversions only:** how much of a material one unit of a
   product contains (kg per m, the workbook's "Peso kg/m"). There is no
   material-to-material pricing relation.
8. **No price breaks.** The table is empty in production and would be a final-price
   override that skips the margin. It gets dropped.
9. **Current production data is the source of truth**, not the workbook. Unimported
   workbook families (ELECTRACLEAN, Fire Blanket, the ALUMOCLAD outlier) are out of
   scope.
10. **Fix CCA's weight column.** It holds kg/km (18.11 for 14 AWG matches copper at
    ≈18.5 kg/km) in a column that means kg/m.
11. **Staleness hint:** show "actualizado hace N días" next to each material price, on
    `/ajustes` and in the builder. It's a warning only and never blocks issuing.
12. **No scheduled/future-dated publishing.** An edit takes effect when it's saved.

### Model

```
margin_options   (id, name, value_micros, sort, is_default, retired_at,
                  created_at, updated_at, updated_by)       -- audited
materials        (id, name, unit_id → units, price_micros,  -- e.g. "CCS 30%", kg, 155
                  updated_at, updated_by)                    -- audited
product_materials(product_id, material_id, qty_per_unit_micros)
                  -- conversion constant: amount of material per 1 product unit,
                  -- e.g. 0.1723 kg of CCS 30% per m of ALAMBRE 4
quotes.margin_option_id → margin_options
```

- **Product cost (MXN)** = `cost_micros` (flat cost, e.g. CCA, ABASTILUM) + Σ over
  `product_materials` of `qty_per_unit × material.price` (e.g. CCS & AC). Most products
  use one term or the other; allowing both covers "material + labor" without a schema
  change.
- **Units:** `product_materials` quantities are per the product's own base unit
  (`products.unit_id`). A line entered in another unit (rollo) is already converted to
  the base unit by `product_unit_conversions` before pricing, so the two conversions
  compose without special cases.
- **The pricing engine shrinks** to: product cost → `/ (1 − margin)` → line total. No
  currency branch, no break lookup, and no field-presence dispatch between flat price,
  cost, and weight. That dispatch was the source of the footnote-10 CCS bug.
- **Retiring an option:** set `retired_at` instead of deleting, since issued quotes'
  snapshots and `audit_log` still name it. A draft whose option was retired shows an
  error ("El margen elegido ya no está disponible; elige otro") and can't be issued
  until a new option is picked.
- **Default option:** new quotes start on the option flagged `is_default`, and the
  vendedor can change it in the builder header.
- **Revisions:** `CreateRevision` copies `margin_option_id` from the original. Because
  drafts follow, a revision reprices live at the current option value and material
  prices, same as today.
- **Freezing at `Emitir`:** `pricing_inputs` records the option id, name, and value; each
  material's id, name, price, and qty-per-unit; and the flat cost. That makes "which
  margin and what material price did we send in March" answerable from the quote
  alone. The PDF still never shows a margin.
- **`/ajustes`** becomes two admin-only lists: Márgenes and Materiales. The `settings`
  table holds no pricing inputs anymore; keep it for future non-pricing settings (e.g.
  default validity days).
- **Being dropped:** `products.unit_price_micros`, `products.currency`,
  `quotes.currency`, `quotes.fx_rate_used_micros`, the `price_breaks` table,
  `pricing.Settings`, and the `fx_rate`/`copper_price`/`default_margin` keys. The 6
  issued quotes lose their `fx_rate_used_micros` value. None of their lines were USD,
  so it never affected a price, their PDFs are stored bytes, and `audit_log` keeps the
  old setting's history.
- **`cmd/import`:** retire it. It was a one-shot migration from the workbook, already
  run, and production data is now the source of truth (decision 9). Updating it to the
  new model would keep a second, stale path into the catalog alive for no use.

### Migrating production data without silently moving prices

- **Seed the menu with exactly four options (user, 2026-09-22):** 12.34% (default;
  today's CCA margin), 16.56%, 20.28% (the workbook's other two CCA columns), and 29.55%.
  Names are set at 3.1. Two price effects are accepted, not special-cased:
  - **CCS & AC now carries a margin.** Production priced it at `kg/m × copper_price`
    ($160) with no margin; it now prices at `kg/m × 160 / (1 − margin)`, e.g. ALAMBRE 4
    $27.57 → $39.13 at 29.55%. (Against the workbook's `kg/m × 220`, a $155 cost at
    29.55% would have landed within 2 centavos; the engine test still checks that.)
  - **ABASTILUM has no 20% option.** Its costs are backed out at the margin that
    actually produced them (below), so the stored costs are true costs. Lighting quotes
    then price at whichever option is picked: at 20.28% about 0.35% above today (a
    $2,100.00 poste → $2,107.38), and at 12.34% well below.
- **CCS & AC:** create the material "CCS 30%" at $160/kg, and one `product_materials`
  row per product from its current `kg_per_m_micros`.
- **ABASTILUM:** back out `cost_micros` from each stored flat price with the margin that
  produced it: `price × (1 − 20%)`, or `× (1 − 12.34%)` for LEDVANCE. Postes keep the
  FX 18 that's already inside their MXN price, so the result is their MXN cost (e.g.
  1680 × 18 = $30,240). Do this in SQL rather than re-importing, for the same reason as
  the CCS/AC spot-fix in footnote 10: hand edits since import must survive. Dry-run it
  against production first, and cross-check against the workbook's `B` column.
- **CCA weights:** `kg_per_m_micros / 1000` on CCA products (dividing keeps any hand
  edit). No price moves, because CCA prices from flat cost.
- **Production quotes are test data (user, 2026-09-22).** All 9 quotes (3 drafts, 6
  issued) are mock data, so no migration has to preserve them. Default: the 3 drafts get
  `margin_option_id` by prefix (QA → 12.34%, QS → 29.55%, QI → 20.28%). If carrying
  quotes forward complicates any slice, wiping `quotes`, `quote_lines`, their stored PDFs
  and the folio sequences is acceptable instead. That's still a production delete, so it
  needs the user's go-ahead when it happens. The products catalog is real data and is
  always preserved. Issued quotes aren't touched beyond the column drops above.
- **Accepted consequence:** a mixed quote (e.g. postes at 20.28% plus LEDVANCE at 12.34%)
  can't reproduce today's per-product mix, because one quote now has one margin. That's
  intended.

### Open questions

None blocking. The seeded options' display names are the user's call at 3.1.

### Slices

| # | Slice | Owner | Done when |
|---|---|---|---|
| 3.1 | `margin_options` (seeded) + `quotes.margin_option_id`; margin dropdown in the builder header; CCA prices from the quote's option; admin "Márgenes" list on `/ajustes`; retire flow | me | Two QA drafts with different options price differently; editing an option reprices a draft that follows it; production drafts unchanged on deploy — ✅¹² |
| 3.2 | `materials` + `product_materials`; CCS & AC costed from material × kg/m and priced through the quote margin; `copper_price` setting removed; admin "Materiales" list with staleness hint; CCA weights ÷ 1000 | me | Under the 29.55% option, every CCS & AC unit price equals `kg/m × 160 / 0.7045`; changing the CCS 30% price reprices CCS drafts — ✅¹³ |
| 3.3 | ABASTILUM backed out to `cost_micros`; drop `unit_price_micros`, currencies, FX, `price_breaks`, and `pricing.Settings`; retire `cmd/import` | me | Every ABASTILUM `cost_micros` equals the workbook's `B` column (postes × 18); `grep -ri "usd\|fx_rate\|price_break"` finds only migrations; issued quotes still reprint byte-identically — ✅¹⁴ |
| 3.4 | Products CRUD edits flat cost + material composition; `pricing_inputs` snapshot extended | me | Folded into 3.2 (¹³): once CCS priced from materials, the product page had to show and edit them in the same slice |

Each slice updates `docs/guia/` (administración, cotizaciones, productos) in the same
commit.

¹² `migrations/0007_margin_options.sql` adds `margin_options` (name, `value_micros`,
`is_default` with a partial unique index, `retired_at`), audit triggers for it, and
`quotes.margin_option_id` plus `margin_name_snapshot`/`margin_snapshot_micros`. It
recreates the three quotes audit triggers so a margin change is audited. It seeds the four
options without the percentage in their names (**Estándar** 12.34% as default, **Medio**
16.56%, **Alto** 20.28%, **CCS & AC** 29.55%), since a name like "12.34%" goes stale the
first time an admin edits the value; the UI always prints the value next to the name. It
also backfills drafts by series and deletes `default_margin`. `pricing.Settings.Margin` is
now a pointer, and a nil margin fails only cost-priced lines (`pricing.ErrNoMargin`); flat
and weight-priced lines still price. The web layer resolves a draft's margin
(`resolveMargin`) from the submitted dropdown value, falling back to the saved option, so
changing the dropdown reprices through htmx before anything is saved. The margin error
renders inside the swapped line fragment, so picking a valid option clears it. `Guardar`
and `Emitir` save the option with the lines (`ReplaceQuoteLines` gained a
`marginOptionID` argument). `Emitir` refuses (422) a missing or retired option. `IssueQuote`
freezes the option's name and value, and `pricing_inputs` records `margin` and
`margin_option` for cost lines. Admins add, rename/revalue, make default, retire and
restore options on `/ajustes` (`POST /ajustes/margenes[/{id}[/predeterminado|retirar|restaurar]]`).
The default can't be retired, and a retired option can't be the default. Percentages are
entered as typed (`12.34` or `12.34%`, at most 4 decimals, < 100).

Verified with `go test ./...` (store lifecycle, audit attribution, migration backfill over
pre-0007 data, and web tests for per-draft pricing, following an edited option, a retired
option blocking issue, and the issue-time freeze) and end to end in a real browser against
a copy of the old local dev DB, which ran 0003–0007 cleanly. On `cca-c14` (cost $5.54),
Estándar, Medio and Alto priced at $6.32, $6.64 and $6.95, exactly the workbook's three
"Cladex 12.34% / 16.56% / 20.28%" columns. Revaluing Alto to 25% moved the saved draft to
$7.39. Retiring it showed the error and the "— retirado" entry. Switching back to Estándar
cleared the error in place, and the issued quote shows "Margen: Estándar (12.34%)". Its
PDF has no margin in it.

¹³ `migrations/0008_materials.sql` adds `materials` (name, fixed `unit_id`, pure-cost
`price_micros`, `updated_at` for the staleness hint) and `product_materials` (product,
material, `qty_per_unit_micros`, unique per product and material), both audited. It
seeds **CCS 30%** at $160/kg (decision 6) and gives every CCS & AC product its kg/m as
CCS 30% content. It clears any `cost_micros` left on those products: that's the stale
pre-M2.3 "cost before margin", which would otherwise now count the material twice.
Production had none, but the old local dev DB did, and that's how this turned up. It
also divides CCA `kg_per_m_micros` by 1000 and deletes `copper_price`.

`pricing.Product` loses `KgPerMMicros` and gains `Materials`, and `Settings` loses
`CopperPrice`. `Product.Cost()` is the flat cost plus each material's qty × price, and
anything with a cost prices at cost / (1 − margin). `products.kg_per_m_micros` stays as
reference weight only ("solo informativo" in the form). `computeQuoteLines` loads each
product's materials. `pricing_inputs` now records `cost`, a `materials` array (name,
qty per unit, unit, price), `margin` and `margin_option`, replacing `kg_per_m` and
`copper_price`. The builder fragment lists the materials its lines use, with price and
age ("CCS 30% $160.00/kg (actualizado hoy)"). `/ajustes` drops the copper field and gains
a Materiales list, admin-only (`POST /ajustes/materiales[/{id}]`): rename, reprice, add.
There's no delete, and the unit can't change after creation. The product edit page gains
a Materiales section (`POST /productos/{id}/materiales[/{mid}/eliminar]`, same permission
as the rest of the product page). Its sections now travel as one
`views.ProductEditSections` rather than more positional arguments.

Before pushing this time, the migration was dry-run against a `.backup` copy of the
**live** production DB (the nightly backup predated 0007). That dry run is how the
production copper_price of $160 surfaced, which led to decision 6. It applied cleanly:
9 CCS & AC products got content, integrity and foreign-key checks passed, and the copy
was deleted afterwards.

Verified with `go test ./...` (engine: materials, flat + materials, CCS against the
workbook; store: CRUD, product scoping, the 0008 migration over pre-0008 data including
a stale CCS cost; web: a CCS draft at $39.13 with the hint, following a material price
change to $48.91, the issue-time materials snapshot, the admin and product-page actions
with their errors). Also smoke-tested against a fresh copy of the local dev DB: CCS
ALAMBRE 4 at $39.13, CCA c14 unchanged at $6.32, and CCA weight 0.01811. The two new
sections were checked visually in the browser.

¹⁴ The live-DB dry run changed the plan again. Production's ABASTILUM rows already held
the workbook's **raw costs** (`cost_micros`, no flat prices): they were entered under a
different SKU scheme from the importer's, so the planned back-out had nothing to do.
They had also been priced as cost / (1 − margin) since 3.1. The postes' costs are the
workbook's USD figures stored as MXN, so a 4 m poste quoted at about $2,107 instead of
about $37,800. The user chose (2026-09-22) to convert them at the workbook's frozen 18.

`migrations/0009_drop_fx_currency_flat_prices.sql` handles both catalog shapes:
- It multiplies raw-cost postes by 18.
- It backs out any flat ABASTILUM price the old importer left (× (1 − 12.34%) for
  LEDVANCE, × 0.8 otherwise). Production has none, but the old local dev DB does, and
  it converges to the same poste cost ($30,240).
- It deletes `fx_rate` and drops `price_breaks`, `products.unit_price_micros`,
  `products.currency`, `quotes.currency` and `quotes.fx_rate_used_micros`. The products
  and quotes audit triggers name those columns, so they're dropped first and recreated
  without them (SQLite refuses to drop a column a trigger references).

`internal/pricing` is now `UnitPrice(Product{CostMicros, Materials}, margin)`, with
`ErrNoCost`/`ErrNoMargin` and no settings, FX, flat prices or price breaks.
`quotes.go` resolves only the margin, and `pricing_inputs` always records `margin` and
`margin_option`. The product form loses Moneda and Precio unitario, and the list's
Precio/Moneda columns become one Costo column. `/ajustes` loses its settings form and
`POST /ajustes` (only margins and materials remain; the `settings` table stays for
future non-pricing settings). `cmd/import` is deleted (excelize left `go.mod`). Every
price is MXN, and the guide says so.

Verified with `go test ./...`: the engine, TestMigration0009 over both catalog shapes,
the reprint staying byte-identical after margin, cost and customer-name changes, and
/ajustes no longer showing FX. The dry run on a `.backup` of the live DB: 9 poste costs
×18, the other 33 ABASTILUM rows and every other family unchanged, integrity and
foreign-key checks ok, 0 USD products or quotes, 0 price breaks. Also smoke-tested on
the old-format dev DB: the 4 m poste quotes at $37,932.76 at 20.28%. Each migration that touches production data gets a dry run against production
and the user's go-ahead before it runs for real (see `docs/NAS_OPERATIONS.md`).

---

# Backlog — deprioritized, not scheduled

**LLM-assisted RFP ingestion.** Parse inbound quote requests (PDF, scan, photo, pasted
email) into a standardized structure that pre-fills a *draft* quote for human review.
Two-stage pipeline (Stage A extraction with structured outputs → Stage B catalog matching);
schema drafted and validated; `quote_lines.source` and `rfp_extraction_id` columns already
reserved in the M0.2 schema. Blocked on collecting real RFPs and Anthropic credentials.
Revisit once M2 is in daily use.

**Auth hardening**: OAuth/SSO, 2FA. Username+password is adequate for 5 known users behind
a rate-limited login; revisit if headcount or exposure grows.

**Finer-grained RBAC and a mutation-approval workflow.** Flagged by the user when
provisioning the first real accounts (2026-08-12): the actual org has more than two
tiers — sysadmin (full/root), admin (COO-level, broad write), and sales (narrower:
read plus specifically the factura flow) — where today's schema only has `admin` and
`vendedor`, and `vendedor` already gets full read/write on products, customers, and
quotes (only `/usuarios`/`/ajustes` are admin-gated). The four initial accounts were
provisioned on the existing two-role schema as a stopgap (sysadmin+COO → `admin`,
both sales → `vendedor`), not because that's the right long-term model. Two related
ideas to design together, not separately: (1) a request/approval system so lesser
roles can submit mutations for a higher role to approve rather than being flatly
denied: (2) **facturas** (invoices — distinct from quotes/cotizaciones; Mexican
CFDI/tax documents) created as mutable drafts, then frozen as immutable once
published — this is structurally the same draft→frozen pattern the quotes engine
already uses (`quotes.status` `borrador`→`emitida`, see M2 above), so whether
facturas reuse that machinery or need their own is worth resolving before
either is built. Revisit once M2 (quoting) is in daily use and it's clear which
CRUD/quoting patterns are actually solid enough to extend.

**Email intake** for RFPs (IMAP polling on the Cladex domain). Depends on the above.

**Other**: htmx round-trip limits on the quote builder → JS island, decided in 2.2. TLS
terminates on the VPS; fix if ever needed is FRP raw-TCP passthrough.
