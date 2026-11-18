# Leads y listas de precios

**Draft 2026-11-18; Gerry decided margins go on the tipo de cotización, next to its terms. Questions for Emilio posted on #82 the same day. Nothing built.** Driven by
[#82](https://github.com/gerrygoo/cladex-web/issues/82): Emilio wants to give prices to
people who aren't a real proyecto yet, without creating proyectos that pollute Pronóstico
(today: `QA0046 · Cotizaciones informativas de producto`, later marked Perdido), and still
follow them up.

The issue bundles four features. They share little code, so this proposes four slices,
each shippable alone, with the open questions for Emilio and the decisions for Gerry.

## 1. What exists today

- **Margins** (`margin_options`, migration 0007) are global: four named options (Bajo
  12.34%, Estándar 16.56%, Medio/Alto 20.28%, Alto 29.55%), one default, retired rows kept
  because issued quotes and the audit log name them. A quote stores `margin_option_id`,
  `margin_name_snapshot`, `margin_snapshot_micros` and `custom_margin_micros`
  (**Otro margen** already exists for QA, QS, QI and QG; see `docs/guia/cotizaciones.md`).
  The CCS copper-price rule (`quote_types.copper_price_margin`) is a separate mechanism.
- **Familia vs. tipo de cotización** (`docs/planning/tipos-de-cotizacion.md`): terms,
  letterhead and the folio counter live on `quote_types`; `product_families` is the catalog
  category. The issue says "familia" throughout, but terms are per *tipo* today. Most
  familias map 1:1 to a tipo (QCCA ↔ CCA, QCCS ↔ CCS, QEXL ↔ Exceline, QAST ↔ Astro).
- **Vigencia** is `quotes.valid_until`, typed by the vendedor, required to issue. There is
  no per-familia rule.
- **A quote becomes a proyecto when issued.** `quotes.project_id` is NULL on a draft and
  set at issue; one live quote per proyecto. Pronóstico, avisos (#69) and dashboards all
  hang off `projects`.
- **Customers** need only a name: RFC, razón social, régimen and CP are optional until a
  factura is made, which already validates them. So "cliente oficial = when invoicing" is
  mostly how it works now.
- **Avisos de seguimiento** (#69 A/B) run on `projects.next_follow_up` with per-user
  toggles and `aviso_rounds`.

## 2. Slices

| | Slice | Schema | Risk |
|---|---|---|---|
| L1 | **Márgenes por tipo de cotización** | `margin_options.quote_type_id` (nullable = global), seeded by copying the 4 current options to every tipo | Pricing: every new quote's margin list changes |
| L2 | **Leads**: alta, lista, seguimiento with avisos, Perdido with motivo, **Convertir en proyecto** | New `leads` table; `lead_id` on projects | Avisos code gets a second subject |
| L3 | **Listas de precios**: PDF of a whole familia at one of its margins, with vigencia and T&C; history on the lead | New `price_lists` (snapshot rows + PDF), per-tipo `validity_rule` | New document; no folio |
| L4 | **Cotización de referencia** tied to a lead, not a proyecto | Quotes with `lead_id` instead of `project_id` | Touches the quote→proyecto state machine |

Recommended order: **L1 → L2 → L3**, then decide whether L4 is needed at all. A price
list (L3) covers most "send me prices" requests. When the lead brings concrete quantities,
it's arguably a real proyecto and **Convertir en proyecto** + a normal quote (at low
probability) already covers it. L4 is the costly one.

### L1 · Márgenes por familia

- Add `quote_type_id` to `margin_options` (NULL = the current global four, kept for history).
  The migration copies the four current options to each tipo, with the same names and
  values, so on day one nothing changes in price.
- **Configuración del sistema → Márgenes** gets a familia selector; each familia keeps
  exactly four active options plus its default. The admin edits names and percentages per
  familia.
- The quote builder lists the margins of the **quote type's default familia**; Otro margen
  stays for every type (today it's only QA, QS, QI and QG, so this extends it).
- Issued quotes are untouched: they carry snapshots.
- **Decision for Gerry:** key margins on *familia* (as asked) or on *tipo de cotización*
  (where terms and letterhead live). A quote has a tipo, not a familia; a mixed quote (QA
  with products from several familias) has no single familia. *Recommendation: key on
  tipo de cotización* and label it with the familia's name in the UI. QA, QL and QI keep the
  global four.

### L2 · Leads

- `leads`: contact name (required), company (optional), phone and/or email (at least one),
  vendedor (`user_id`), `next_follow_up`, status `abierto | perdido | convertido`, perdido
  motivo, `project_id` once converted, optional `customer_id` when the company already
  exists.
- New section **Leads** (list + page with history and comments, like a proyecto's).
  Leads never show in Proyectos, Pronóstico or the dashboards' pipeline.
- Seguimiento: the #69 A/B avisos and the pendientes dashboard get leads as a second
  subject. Comments with a new date, Perdido (motivo, no date), or conversion stop them.
- **Convertir en proyecto** creates (or picks) the customer from the lead's company and
  contact, and opens the new-quote flow for it. The proyecto shows "Viene del lead …" and
  the lead's history stays readable from there.

### L3 · Listas de precios

- Pick a familia and one of its margins (or Otro margen) → PDF with every active product
  of the familia at that margin, its date, **vigencia** and the tipo's T&C (with delivery
  times). No sale folio.
- Stored as a snapshot (`price_lists` + lines + PDF) so the history on the lead says what
  was sent: familia, margin, date, who.
- Vigencia rule per tipo, editable in Catálogos: *end of the issuing month* (CCA, CCS) or
  *N days*. Astro and Exceline are pending from Emilio.
- Can a price list be made without a lead, e.g. for a customer? Probably yes (lead
  optional), but the issue only asks for leads.

### L4 · Cotización de referencia (optional)

A quote issued against a lead: same builder, own folio, PDF, not a proyecto, not in
Pronóstico. That means quotes can be issued without creating a proyecto, a
`lead_id` on quotes, and conversion re-pointing the quote at the new proyecto. It changes
the quote state machine, so it's the biggest slice. Defer until L2/L3 have been used.

## 3. Questions for Emilio

1. **Astro and Exceline vigencia**: end of month, N days, or something else?
2. **Lists by familia or by tipo de cotización?** E.g. is "Exceline" one list, and are
   CCS and CCA always separate lists?
3. **What a price list shows**: price per meter (and per kilo for CCS?), currency (MXN, or
   USD for some familias?), and is copper-price adjusted pricing (CCS) included?
4. **Reference quotes**: is a price list plus "convert to proyecto when they bring
   quantities" enough, or do you need quotes with specific products for leads too?
5. **Who sees leads**: each vendedor only their own (like Pronóstico), admins all?
6. **Existing customers**: can a lead be someone from a company that's already a customer
   (a new contact)?

## 4. Decisions

1. **Margins on the tipo de cotización**, next to its terms (Gerry, 2026-11-18), labelled
   with the familia's name where that reads better. QA, QL and QI keep the global four.
2. Order L1 → L2 → L3, with L4 deferred until the others have been used.
3. The §3 questions went to Emilio on #82 on 2026-11-18.
