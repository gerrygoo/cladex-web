# Track 1 · Cotizaciones

The original product: replace the Excel cotizador with deterministic pricing, numbered
quotes and a PDF. Everything else in the app hangs off an issued quote.

## As built

- **Pricing** in code from the database, never from a model (`docs/PLAN.md`, pinned
  constraint 1). Micro-pesos per unit, centavos per total, rounding once.
- **Tipos de cotización** (`quote_types`) own the prefix, counter, terms, letterhead,
  vigencia rule and the builder's behavior flags. **Familias** are the catalog only.
  ([tipos-de-cotizacion.md](../planning/tipos-de-cotizacion.md))
- **Margins** per tipo (four named options plus Otro margen), CCS priced from the
  copper price per kilo, QG per kilo, QL with manual cost and margin per free line and
  the supplier's quote attached.
- **Revisions** (`-R1`) replace the quote on the same proyecto; issued quotes are
  frozen snapshots (lines, margin, terms, letterhead).
- **PDF** through Typst, named `<folio>_<cliente>.pdf`.
- Duplicate a quote for any customer; delete a draft or a revision made by mistake.

## Signals

- **Most of the work so far**: about 96 of 317 commit subjects touch quoting, pricing,
  margins or folios, and most of the first 60 issues were quote builder and PDF
  adjustments. That stream has slowed: November's issues are mostly about proyectos,
  operación and leads.
- **Repeated pull toward per-tipo behavior**: logos per tipo (#16, #47, #48), terms per
  tipo (#58), margins per tipo (#82 L1), vigencia per tipo (#82). Every new familia so
  far has needed a code change somewhere.
- **FX was removed twice** (migrations 0009, then a currency label only in 0015). USD
  exists as a label on QL; costs are MXN.
- Volume is small: ~25–30 quotes a month, ~11 lines each.

## Growth edges

| # | Edge | Trigger | Functional | Non-functional | Size | Depends on |
|---|---|---|---|---|---|---|
| C1 | **Send the quote from the app** | Vendedores attach PDFs to emails by hand; we want to know a quote reached the client | "Enviar" on an issued quote emails the PDF to the customer's contacts, with copies; the history shows what was sent, when, to whom | Needs attachments on notifications (planned for N5). Sending must not change the snapshot. Bounces visible | S / Internal / high | Notifications N5 attachment column ([plataforma](plataforma.md)) |
| C2 | **Margin guardrails and approval** | A quote leaves below a floor margin, or Otro margen gets used to discount without anyone noticing | Issuing a quote under a tipo's floor margin asks an admin to approve it; admins see pending approvals on Equipo | First real use of the approval workflow the RBAC backlog item imagines. Approvals must be audited | M / Decision / med | Roles (P1) |
| C3 | **Stale-price warning on open quotes** | The copper price or a product cost changes while prospectos still carry old prices | A prospecto whose quote would price differently today shows the difference and offers a revision | Compute on demand per proyecto (derived, no stored flag). Reuses `computeQuoteLines`; cost of re-pricing ~11 lines is trivial | S–M / Internal / med | — (`costChangeNote` already diffs costs between a quote and its revision; this compares against today's catalog instead) |
| C4 | **New tipo de cotización without code** | Each new familia (Exceline, Astro) needed letterhead and pricing tweaks in Go | An admin creates a tipo, uploads its logo, picks its pricing mode (per unit, per kilo, per metre with kilos, free lines) and it works | Remaining behavior switches in code become data; letterhead images become uploads (files, see P4) | M / Internal / med | Uploads policy (P4) |
| C5 | **RFP ingestion** (LLM pre-fills a draft) | Quote requests arrive as PDFs and photos and take time to type | Drop a request; get a draft with matched catalog lines, every match marked for review | **No model output reaches a price** (pinned). Anthropic credentials as a Compose secret. Per-extraction cost and latency (seconds, so a job). Needs a corpus of real RFPs first | XL / Expert (real RFPs) / low | Jobs (built); a collection of real requests |
| C6 | **USD quotes with an exchange rate** | A supplier bills in USD and the margin moves with the peso | A quote in USD with the rate captured at issue | Snapshot the rate. Twice removed: needs a strong reason. Interacts with CFDI (moneda, tipo de cambio) | M / Decision / low | — |
| C7 | **Quote builder speed** | The htmx round-trip per line feels slow on long quotes | Builder edits feel instant | The backlog's escape hatch: a JS island for the line table. Only if measured | M / Internal / med | — |

## Notes

- C3 is cheap and protects margin directly; C1 is cheap once N5's attachments exist and
  also unblocks supplier O.C. sending (S2) and factura resend (F4). Those three share one
  piece of plumbing.
- C5 has been in the backlog since August. Its blocker is not code but a corpus: start
  saving real RFPs (they already arrive; the QL supplier attachments are a similar flow)
  well before building.
