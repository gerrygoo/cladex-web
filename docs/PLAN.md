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

## Quote persistence

Every generated quote persists permanently. The spreadsheet has no history, so nobody can
answer "what exactly did we send Grupo PEME in March, and at what copper price?"

1. **Issued quotes are immutable.** `quote_lines` snapshot `description`,
   `unit_price_micros`, and the inputs that produced it (`kg_per_m`, `margin`,
   `metal_price`, `fx_rate`) as JSON. Nothing reads live prices when reprinting.
2. **Revisions are new rows.** A changed quantity creates a new quote with
   `supersedes_quote_id` and folio suffix (`QA0105-R1`). The original is untouched.
3. **The rendered PDF is stored, not regenerated.** Bytes to
   `/volume1/docker/cladex/data/quotes/<folio>.pdf`, SHA-256 in the row. The Typst template
   *will* change; this is the only way a reprint years later is byte-identical to what the
   customer received.

**Tables**: `quotes` (`folio` unique, `prefix` QA/QS/QI, `customer_id`, `user_id`, `status`,
`currency`, `fx_rate_used`, `subtotal`, `iva`, `total`, `terms_snapshot`, `created_at`,
`issued_at`, `valid_until`, `supersedes_quote_id`, `pdf_path`, `pdf_sha256`) and
`quote_lines` (`quote_id`, `line_no`, `product_id` **nullable** — the "Cotizador libre"
sheet proves free-text lines are real — `description_snapshot`, `qty`,
`unit_price_micros`, `line_total`, `pricing_inputs` JSON, `source`, `rfp_extraction_id`).

Drafts are mutable; setting `issued_at` freezes the row and writes the PDF.

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
| 1.1 | Backups: nightly `sqlite3 .backup` + the `quotes/` PDF dir, offsite copy | me¹ | Restore into a scratch dir and open it — **before 1.2 loads real data** |
| 1.2 | `cmd/import` — parse the 4 catalog sheets into families, products, price_breaks | me | Idempotent; row counts match; spot-check 5 SKUs by hand |
| 1.3 | Auth: bcrypt, sessions table, login/logout, middleware, rate limiting, CSRF | me | Log in, hit a protected route, log out → 302. 6th bad password is throttled |
| 1.4 | `cladexctl user add/passwd/disable` + change-own-password page | me | Create a user via CLI, log in as them, change the password, log in again |
| 1.5 | Products CRUD — list, search, create, edit, soft-delete | me | Full lifecycle through the UI; persists; works with JS disabled |
| 1.6 | Customers CRUD | me | Same |
| 1.7 | Users admin + settings (FX, metal prices, margins), admin-gated | me | Vendedor gets 403 on both; admin can edit FX and see it reflected |

**1.2 is the riskiest slice.** The sheets are irregular — merged headers, a hidden
`Descripción | Precio` block on the right of each, per-family layouts that disagree.

## M2 — Quoting engine and PDF

| # | Slice | Owner | Done when |
|---|---|---|---|
| 2.1 | Pricing engine, pure Go, no HTTP: `kg/m × metal $/kg × (1+margin)`, USD×FX, qty breaks, IVA | me | Unit tests reproduce ≥15 known prices from the spreadsheet exactly |
| 2.2 | Quote builder UI — htmx shell + vanilla-JS island for live line editing and totals | me | Build a 5-line quote end to end; totals match 2.1 |
| 2.3 | Typst cotización template + render pipeline; per-family terms blocks | me | Generated PDF matches the current Excel output for the same input |
| 2.4 | Folio sequences, issue flow (freeze row, write PDF + SHA), status, revisions | me | Issue, change FX and copper, reprint → byte-identical PDF. Revise → `-R1`, original untouched |
| 2.5 | Quote history: list, filter by customer/vendedor/status, re-download stored PDF | me | Every quote ever issued is findable and re-downloadable |

**2.1 before 2.2, without exception.** The engine is testable against the spreadsheet's own
numbers; the UI is not. Prove the engine first and later UI bugs can never be mistaken for
pricing bugs.

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

**Email intake** for RFPs (IMAP polling on the Cladex domain). Depends on the above.

**Other**: htmx round-trip limits on the quote builder → JS island, decided in 2.2. TLS
terminates on the VPS; fix if ever needed is FRP raw-TCP passthrough. Stored PDFs grow
`data/quotes/` unboundedly — harmless for years, but 1.1's backup must cover it.
