# Baseline: what production looks like (2026-11-25)

Measured read-only on the NAS on 2026-11-25 (`sqlite3 -readonly`, aggregate counts
only). Real use starts **2026-10-20**: the first quote in the current database is from
that day, so the rates below cover about five weeks. Re-measure before any forecast
that leans on them; the commands are at the end.

## People

4 users (2 admins, 2 vendedores, per `docs/PLAN.md`'s backlog). Emilio is the business
owner and the source of almost every issue.

## Volume

| Record | Count | Rate (Oct 20 → Nov 24) |
|---|---:|---|
| Quotes | 39 (33 emitida, 5 revisada, 1 borrador) | 12 in late Oct, 27 in Nov, about **25–30 a month** |
| Quote lines | 421 | about 11 per quote |
| Proyectos | 33 (27 prospecto, 1 en curso, 5 perdido) | 9 in late Oct, 24 in Nov |
| Customers | 35 | |
| Products | 210 | step changes from bulk loads (Exceline 60, Astro 80) |
| Leads | 4 | since 2026-11-18 |
| Price lists | 2 | |
| Quote comments | 93 | about 3 per proyecto |
| Pendientes libres | 12 | |
| Audit log rows | 2,169 | about **1,800 a month** |
| Jobs / notifications | 147 / 90 | since 2026-10-26 |
| Supplier O.C. / payments / comprobantes | 1 / 1 / 0 | operación has barely started |

Reading this: the commercial side is in real use; operación (O.C., entrega, factura,
cobro) has one proyecto in it. Every forecast for tracks 3 and 4 is a forecast about
use that hasn't happened yet, so it has low confidence until a few dozen proyectos have
gone through en curso.

## Storage

The database is 92 MB (plus a 7 MB WAL). Almost none of it is business data:

| Table | MB | What |
|---|---:|---|
| `official_documents` | 74.3 | fichas técnicas and presentations, uploaded once |
| `supplier_quote_files` | 9.2 | QL supplier quotes (#39) |
| `project_files` | 1.6 | client O.C.s, delivery and payment receipts |
| `audit_log` | 1.3 | |
| everything else | < 2 | quotes, proyectos, catalog, jobs… |

Backups: a nightly tarball of about 74 MB, kept 14 days (about 1.1 GB), plus the
Litestream file replica (1.1 GB). The volume has 11 TB free, so disk is not the
constraint. Offsite copy relies on UGREEN's NAS-to-cloud sync, which
`docs/NAS_OPERATIONS.md` describes as deferred: **check whether it is set up**.

## Forecast: 12 months

Two scenarios: **×1**, today's rate, and **×5**, which is roughly what leads in daily
use, a third vendedor and operación at full use would add up to.

| | ×1 | ×5 | Where it first matters |
|---|---:|---:|---|
| Quotes | ~350 | ~1,800 | Nowhere: SQLite on the N100 handles this without effort |
| Quote lines | ~4,000 | ~20,000 | Nowhere |
| Proyectos | ~300 | ~1,500 | Lists paginate **in memory** (`internal/web/paginate.go` loads every row, then slices), and each proyecto row runs per-row subqueries (last comment, states view). Fine at ×1; time it at ×5 |
| Audit log rows | ~22,000 | ~110,000 | Seguimiento periods are **replayed from the audit log** on every Mis pendientes / Equipo load (`docs/planning/dashboard.md` decision 2). First thing to time |
| Uploaded files | +20–50 MB | +100–250 MB | Every nightly backup copies the whole DB, blobs included |
| Comprobantes (XML) | ~100 | ~500 | Size is nothing; the legal retention is what matters (track 4) |

Guesses, labelled as such: files assume about 1 MB per QL supplier quote and per en
curso proyecto, and no new bulk of documentos oficiales.

## Thresholds to watch

Proposed numbers, so "is this slow yet?" has an answer before someone complains:

| Signal | Threshold | Then |
|---|---|---|
| Mis pendientes / Equipo response, p95 | > 300 ms | Cache seguimiento periods in a table, as dashboard.md already allows |
| Any list page, p95 | > 500 ms | Index or paginate that list |
| Database file | > 500 MB, or nightly tarball > 250 MB | Move blobs out of SQLite to content-addressed files under the data dir (plataforma edge P4) |
| Litestream replica | > 5 GB | Review snapshot interval and retention |
| Jobs failed in a week | > 0 unexplained | Already visible on Trabajos; alert on it (plataforma edge P6) |

There is no request-latency measurement today beyond request logs; P6 in the
plataforma track is where that lands.

## Re-measuring

```sh
ssh cladex-nas 'cd /volume1/docker/cladex && sqlite3 -readonly "file:data/cladex.db?mode=ro" \
  "select substr(created_at,1,7), count(*) from quotes group by 1;
   select count(*) from audit_log;
   select name, sum(pgsize)/1048576.0 from dbstat group by name order by 2 desc limit 8;"'
ssh cladex-nas 'du -sh /volume1/docker/cladex/{data,backups}'
```
