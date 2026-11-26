# Track 5 · Catálogo y documentos

The reference data every other track prices, buys and invoices from: products,
familias, materials and their prices, units and conversions, SAT codes, and the
documentos oficiales (fichas técnicas, presentations) the team hands to clients.

## As built

- **Products** (210) with cost or materials × quantity, units and conversions (m ↔ kg),
  SAT product and unit codes (#62), full SAT catalogs embedded (`internal/satcat`).
- **Familias** as catalog categories with a default supplier and a fallback SAT code.
- **Materials** with prices (the copper price per kilo drives CCS).
- **Bulk loads** of Exceline (60) and Astro (80) from Emilio's spreadsheets, each done
  as a **migration** (0038, 0039).
- **Documentos oficiales**: sections, ordering, uploads, admin-edited.
- Products editable by admins only; vendedores can add (#41).

## Signals

- Each new supplier's price list became a one-off code change (#60, #68). That will
  repeat with every new line Cladex carries and every supplier price update.
- Documentos oficiales are **74 MB of the 92 MB database**: one feature dominates
  storage and every nightly backup.
- Prices move (copper, supplier lists); today a price change is an admin editing
  products one by one, or a migration.

## Growth edges

| # | Edge | Trigger | Functional | Non-functional | Size | Depends on |
|---|---|---|---|---|---|---|
| K1 | **Import a price list from a spreadsheet** | The next supplier list or price update (they come several times a year) | An admin uploads an .xlsx/.csv, maps columns, sees a diff (new / changed / missing products), confirms | Validation before writing; one transaction; audited as one import. Issued quotes untouched (snapshots). xlsx parsing in Go (a dependency, or CSV only) | M / Internal / high | — |
| K2 | **Cost history and price-change log** | "When did this cost change, and by how much?" | Each product shows its cost over time; a change feeds C3's stale-price warning | The audit log already holds it; a view over it, not a new table | S / Internal / high | — |
| K3 | **Documents as files, not blobs** | DB or backup thresholds in the [baseline](baseline.md#thresholds-to-watch) | Nothing visible changes | Move uploads to content-addressed files under the data dir; backups and Litestream follow. See P4 | M / Internal / high | P4 |
| K4 | **Attach fichas técnicas to quotes** | Clients ask for the product sheet along with the quote | Pick documents to send with a quote; the email carries both | Uses C1. Large attachments vs email limits (link instead of attach above a size) | S / Internal / med | C1 |
| K5 | **Richer product taxonomy** | Searching 210 products by familia is fine; 1,000 won't be | Sub-categories, attributes (gauge, material), filters | Tipos de cotización deliberately kept familias flat to leave this open | M / Decision / low | Catalog growth |

## Notes

- K1 converts a recurring developer task into an admin task: the clearest "stop doing
  this by hand" edge in the app.
- K3 isn't urgent at 92 MB but is the single biggest lever on backup size; decide the
  file layout once for every upload (supplier quotes, project files, documents).
