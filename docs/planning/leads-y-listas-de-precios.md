# Leads y listas de precios

**Draft 2026-11-18; Gerry decided margins go on the tipo de cotización, next to its terms. Emilio answered the questions on #82 the same day (§3); L4 is dropped. L1 built 2026-11-18 (migration 0047); L2 built 2026-11-18 (0048); L3 built 2026-11-18 (0049).** Driven by
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
| L2 | **Leads**: alta, lista, seguimiento with avisos, Perdido with motivo, **Convertir en proyecto** | New `leads` table; `leads.quote_id` to the converted draft | Avisos code gets a second subject |
| L3 | **Listas de precios**: PDF of a whole familia at one of its margins, with vigencia and T&C; history on the lead | New `price_lists` (snapshot rows + PDF), per-tipo `validity_rule` | New document; no folio |
| ~~L4~~ | ~~Cotización de referencia~~: **dropped** (Emilio, §3.4); the price list with editable quantities covers it | | |

Order: **L1 → L2 → L3**. When a lead brings concrete quantities for a real proyecto,
**Convertir en proyecto** + a normal quote (at low probability) covers it, so there is no L4.

### L1 · Márgenes por familia

- Add `quote_type_id` to `margin_options` (NULL = the current global four, kept for history).
  The migration copies the four current options to each tipo, with the same names and
  values, so on day one nothing changes in price.
- **Configuración del sistema → Márgenes** gets a familia selector; each familia keeps
  exactly four active options plus its default. The admin edits names and percentages per
  familia.
- The quote builder lists the margins of the quote's **tipo**; Otro margen stays as it is
  (every type with catalog products already has it).
- Issued quotes are untouched: they carry snapshots.
- **Decision for Gerry:** key margins on *familia* (as asked) or on *tipo de cotización*
  (where terms and letterhead live). A quote has a tipo, not a familia; a mixed quote (QA
  with products from several familias) has no single familia. *Recommendation: key on
  tipo de cotización* and label it with the familia's name in the UI.
- *As built (0047):* every tipo with catalog products got its own copy of the four;
  QL (solo líneas libres) prices per line and has none. The generales stay, editable, for
  a tipo created later, which uses them until an admin clicks **Darle márgenes propios**
  (copies them and moves its drafts). Drafts moved to their tipo's copy by name; issued
  quotes keep the general option. Revisions and duplicates pick the tipo's option of the
  same name, else its default.

### L2 · Leads

- `leads`: contact name (required), company (optional), phone and/or email (at least one),
  vendedor (`user_id`), `next_follow_up`, status `abierto | perdido | convertido`, perdido
  motivo, `project_id` once converted, optional `customer_id` when the company already
  exists.
- New section **Leads** (list + page with history and comments, like a proyecto's).
  Leads never show in Proyectos, Pronóstico or the dashboards' pipeline.
- **Everyone sees every lead** (like cotizaciones and proyectos today). Avisos de seguimiento
  go only to the lead's vendedor; admins see everyone's seguimientos, as with proyectos.
- A lead can be a **new contact of an existing customer** (e.g. Juan at Sonepar while the
  open proyecto is with Carlos): `customer_id` set, contact name on the lead, so each
  opportunity shows which person it's with.
- Seguimiento: the #69 A/B avisos and the pendientes dashboard get leads as a second
  subject. Comments with a new date, Perdido (motivo, no date), or conversion stop them.
- **Convertir en proyecto** creates (or picks) the customer from the lead's company and
  contact, and opens the new-quote flow for it. The proyecto shows "Viene del lead …" and
  the lead's history stays readable from there.
- *As built (0048):* `leads` + `lead_comments`, `notifications.lead_id`. Conversion asks
  only for the tipo de cotización and creates the draft (owned by the lead's vendedor)
  directly; `leads.quote_id` points at it and the proyecto finds its lead through it.
  Perdido can be reopened. Seguimiento avisos: the owner's steps only, never the team's
  (Emilio's answer 5). Not done: leads in Mis pendientes and Equipo; admins see overdue
  leads in the Leads list (sorted most overdue first) instead.

### L3 · Listas de precios

- **One familia per list, never mixed**: Alumbrado, CCA, CCS, Alumoclad, Exceline, Astro.
  Líneas libres has no list.
- Pick a familia and one of its margins (or Otro margen) → PDF with every active product
  of the familia at that margin, its date, **vigencia** and the tipo's T&C (with delivery
  times). No sale folio. **All in MXN.**
- **Preloaded at quantity 1, editable** (e.g. 1,000 m), with the line total: 1 m of cable
  (plus its kilo equivalent in CCS and Alumoclad), 1 caja in Astro, 1 pieza in Exceline and
  Alumbrado. This replaces the reference quote.
- Unit shown: CCA per metro; CCS and Alumoclad per metro and its kilo equivalent, priced
  from the price per kilo in Configuración del sistema, as their quotes do; Astro per caja;
  Exceline and Alumbrado per unit.
- **The price holds for the vigencia** even if the price per kilo changes later: the list
  stores its snapshot prices.
- Stored as a snapshot (`price_lists` + lines + PDF) so the history on the lead says what
  was sent: familia, margin, date, who.
- Vigencia rule per tipo, editable by the admin: every familia starts at *end of the
  issuing month*; *N days* is the other option for later.
- *As built (0049):* the list is priced by the quote builder's own code: an in-memory
  draft of the tipo goes through `Quotes.resolveMargin` and `Quotes.computeQuoteLines`
  (so tipo margins, Otro margen, CCS kilos from the material price, QG per-kg costs all
  behave as in a quote), and the PDF is the quote layout with `QuoteDocument.PriceList`
  relabelling the header. `price_lists` keeps the rendered PDF (the price holds) plus
  tipo, lead, recipient, margin, vigencia and line count; a lead's list writes an entry
  to its history. Lists are made from a lead's page or from Leads → Listas de precios.
  Vigencia is fixed at end of month for now; the per-familia admin rule is not built.
- Can a price list be made without a lead, e.g. for a customer? Probably yes (lead
  optional), but the issue only asks for leads.

## 3. Questions for Emilio (answered 2026-11-18)

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

Answers (Emilio on #82): 1) end of month for every familia, admin can change it per
familia later; the price holds all month. 2) one familia per list, never mixed. 3) as in
L3 above, all MXN. 4) no separate reference quote: the list has editable quantities, and a
real proyecto goes through Convertir en proyecto. 5) everyone sees all leads; avisos only
to the responsible vendedor. 6) yes, a new contact linked to the existing customer.

## 4. Decisions

1. **Margins on the tipo de cotización**, next to its terms (Gerry, 2026-11-18), labelled
   with the familia's name where that reads better. Every tipo with catalog products gets its own; QL has none.
2. Order L1 → L2 → L3; L4 dropped after Emilio's answer 4.
3. The §3 questions went to Emilio on #82 on 2026-11-18 and he answered the same day.
