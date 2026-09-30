# UI/UX quality

How we judge and track the quality of Cladex's interface. Three tiers:

1. **Compliance** — web standards with pass/fail criteria. A failure is a bug.
2. **Quality** — research-backed heuristics and measurements. A finding is a judgement call,
   prioritised like any other improvement.
3. **Visual design** — hierarchy, order, alignment and consistency, against a small design
   system. The first two tiers pass a page where every button is the same grey; this one
   doesn't.

Scope is the authenticated app (every route in `internal/web/router.go`) plus `/login`.
The PDF is out of scope here; it's a print artifact, not an interface.

## Tier 1 — Compliance (web standards)

| Standard | Body | What it means for us |
|---|---|---|
| [HTML Living Standard](https://html.spec.whatwg.org/) | WHATWG | Use the native element for the job: `<button>` for actions, `<a>` for navigation, `<label for>` on every field, `<table>` with `<th scope>` for tabular data, `<dialog>` for modals. Native elements give keyboard and screen-reader support for free. |
| [WAI-ARIA 1.2](https://www.w3.org/TR/wai-aria-1.2/) | W3C | Only where HTML has no native equivalent (e.g. a combobox, a live region). No ARIA is better than wrong ARIA. |
| [ARIA Authoring Practices Guide](https://www.w3.org/WAI/ARIA/apg/) | W3C (informative) | The expected keyboard behaviour for any custom widget we build, e.g. the product picker. |
| [WCAG 2.2](https://www.w3.org/TR/WCAG22/) level **AA** | W3C (also ISO/IEC 40500) | The pass/fail bar. [Quick reference](https://www.w3.org/WAI/WCAG22/quickref/?currentsidebar=%23col_customize&levels=aaa). |

### Checks specific to our stack (Go + templ + htmx)

- [x] `<html lang="es">` on every page (WCAG 3.1.1).
- [x] Every page has a unique, descriptive `<title>` (2.4.2).
- [x] Landmarks (`<header>`, `<nav>`, `<main>`) and a skip link to `<main>` (2.4.1).
- [x] Field errors are tied to their input with `aria-describedby` and set `aria-invalid`;
      focus starts on the first invalid field (3.3.1, 3.3.3). Use `errAttrs` +
      `FieldError` from `internal/views/a11y.templ`.
- [x] htmx swaps that change content the user didn't directly touch are announced through
      the layout's `#live-status` region (4.1.3). A swapped fragment declares what to say
      with `data-announce`.
- [x] After an htmx swap that replaces the focused element, focus lands somewhere sensible
      and not on `<body>` (2.4.3). Controls inside swapped fragments get stable ids;
      `static/app.js` falls back to the fragment itself.
- [x] Everything works with the keyboard alone, with a visible focus ring (2.1.1, 2.4.7,
      2.4.11). Verified on every screen.
- [x] Text contrast ≥ 4.5:1, UI component and focus-indicator contrast ≥ 3:1 in
      `static/app.css` (1.4.3, 1.4.11), in both colour schemes.
- [x] Click/tap targets ≥ 24×24 CSS px, including table row actions (2.5.8).
- [ ] Usable at 200% zoom and at 320 px width without horizontal page scroll, wide tables
      excepted (1.4.4, 1.4.10). 200% text: nothing clipped. 320 px: everything fits except
      the four filterable list tables (#7).
- [x] Inputs for the user's own data declare `autocomplete` (1.3.5).
- [x] Destructive and final actions ask first: `data-confirm` on the submit button
      (`static/app.js`). Not `hx-confirm`, which only applies to htmx requests.
- [x] Money and quantities have their units in text, not only in colour or position (1.3.1,
      1.4.1).
- [x] Navigating links are `<a>`, never a `<button>` inside an `<a>` (HTML content model).
      Style them with `class="button"`.

### How Tier 1 is checked

- **Automated:** run [axe-core](https://github.com/dequelabs/axe-core) against each screen
  in the preview browser, loaded from a CDN into the page, so there's no npm dependency.
  Axe finds roughly a third of WCAG issues, so it's a floor, not a pass.
  Run it in both colour schemes (the preview pane's `colorScheme` emulation). Snippet:

  ```js
  await new Promise((ok, err) => { const s = document.createElement('script');
    s.src = 'https://cdn.jsdelivr.net/npm/axe-core@4/axe.min.js'; s.onload = ok; s.onerror = err;
    document.head.appendChild(s) });
  const r = await axe.run(document, { runOnly: { type: 'tag', values:
    ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'] } });
  r.violations.map(v => `${v.impact} ${v.id} x${v.nodes.length}`)
  ```

  Audit against a scratch copy of the DB: `cladexctl user add` for a user, then insert a
  `sessions` row with the SHA-256 of a token you pick and set it as the `cladex_session`
  cookie. 320 px reflow: emulate the viewport and compare
  `document.documentElement.scrollWidth` with the viewport width.
- **Manual:** per screen, a keyboard-only pass (Tab/Shift-Tab/Enter/Space/Esc), a 200% zoom
  pass, and a VoiceOver pass (macOS, Safari) on the quote builder and forms.
- **Server-side tests:** where a rule can be asserted on rendered HTML (lang, labels,
  `aria-describedby` on errors, `th scope`), assert it in the existing `httptest` handler
  tests so it can't regress.

## Tier 2 — Quality (heuristics and measurements)

| Reference | Kind | How we use it |
|---|---|---|
| [Nielsen's 10 usability heuristics](https://www.nngroup.com/articles/ten-usability-heuristics/) | Industry heuristics (NN/g) | Heuristic review of each screen. Record findings by heuristic and severity (0–4). |
| [ISO 9241-11](https://www.iso.org/standard/63500.html) | ISO ergonomics standard | Defines usability as *effectiveness, efficiency and satisfaction* for specified users, goals and context. We state task goals that way (below). |
| [NN/g: data tables](https://www.nngroup.com/articles/data-tables/) | Research guidelines | Tables must support find, compare, view/edit one row, and act on rows. Applies to quotes, products, customers and users lists, and the quote lines. |
| [NN/g: web forms](https://www.nngroup.com/articles/web-form-design/), [Baymard: inline validation](https://baymard.com/blog/inline-form-validation) | Research guidelines | Fewest fields possible, labels above fields, validate inline rather than only on submit, keep values on error. |
| [Core Web Vitals](https://web.dev/articles/vitals) | Google metrics on W3C perf APIs | Targets at p75: **LCP < 2.5 s, INP < 200 ms, CLS < 0.1**. Measure with Lighthouse in the preview browser. |
| [System Usability Scale](https://measuringu.com/sus/) | Research questionnaire | Deferred: meaningful from ~20 responses, and we have < 5 users. Revisit if the user base grows. With a handful of users, task metrics tell us more. |

### Heuristic severity scale

| | Meaning |
|---|---|
| 0 | Not a usability problem |
| 1 | Cosmetic — fix if time allows |
| 2 | Minor — low priority |
| 3 | Major — important to fix |
| 4 | Catastrophe — fix before release |

### Core tasks (ISO 9241-11 framing)

For each, the target is measured by watching a seller do it, not guessed.

| Task | Effectiveness | Efficiency | Satisfaction |
|---|---|---|---|
| Create a quote for an existing customer with 5 lines and issue it | Completed without help | Time ≤ the Excel workbook | Seller would rather use it than Excel |
| Find a product by partial code or description | Found on first search | < 10 s | — |
| Change a quote's margin and see the new total | Correct total | ≤ 2 interactions | — |
| Revise an issued quote | Completed without help | — | — |
| Add a new customer from the new-quote flow | Completed without help | — | — |

## Tier 3 — Visual design

### Audience and direction

Sellers and admins in their 60s who are at home with computers and web tools. The look is
corporate, reliable and plain: tried-and-true patterns, done by the book, nothing novel to
learn. For this audience that means larger text and controls, strong contrast, links that
look like links, and one obvious thing to do on each screen.

### References

| Reference | Kind | How we use it |
|---|---|---|
| [NN/g: 5 principles of visual design](https://www.nngroup.com/articles/principles-visual-design/) | Research guidelines | Scale, visual hierarchy, balance, contrast and Gestalt grouping: the vocabulary for findings. |
| [NN/g: Visual hierarchy](https://www.nngroup.com/articles/visual-hierarchy-ux-definition/) | Research guidelines | Each screen's most important element should be the most prominent. |
| Contrast, repetition, alignment, proximity (Robin Williams, *The Non-Designer's Design Book*) | Classic layout checklist | The four questions of the per-screen review below. |
| [NN/g: Designing for older adults](https://www.nngroup.com/articles/usability-for-senior-citizens/) | Research report | Text size, contrast, target size and explicit affordances for users over 60. |
| Design tokens (the variables at the top of `static/app.css`) | Our design system | The only colours, font sizes and control sizes the site uses. |

### Principles

1. **One primary action per screen.** A solid `primary` button for the thing the screen is
   for (Emitir cotización, Guardar, Nuevo cliente). Everything else is an outlined
   secondary button; destructive actions (`danger`) are outlined in red and always ask
   first.
2. **Sections follow the task.** Top to bottom in the order the user works: what it is
   (title, customer, status), the content (lines, totals), what to do with it (save,
   issue), then secondary tools (add lines, comments).
3. **Alignment.** One left edge for content. Numbers (money, quantities) right-aligned
   with tabular figures, and totals under their column. The same button sits in the same
   place on every row.
4. **Proximity.** Things that work together sit together: a search box and its "Buscar",
   a heading and its "?" help, a table and its totals.
5. **Repetition.** Each kind of element looks the same everywhere: buttons, messages,
   status badges, table headers, form fields.
6. **Stable layout, persistent status.** Don't hide and show elements to signal a state;
   show the state. "Hay cambios sin guardar." becomes "Todos los cambios guardados.", not
   nothing, and "Filtros activos" becomes "Sin filtros.". Nothing appears or disappears
   and shifts what's below or to the right under the cursor. When content has to grow
   (adding a line), the control just used stays where it was (`static/app.js`).
7. **Legible for the audience.** 17 px body text that still follows the browser's text
   size, 40 px buttons and fields, underlined links, and every colour at least 5.5:1 on its
   background.

### Design system

**Palette: two brand colours plus five working colours** (`--c-*` at the top of
`static/app.css`). The brand colours come from the logo sheet, `docs/brand/Cladex.svg`,
which is the source of truth for the site's colours and logo (the PDFs' logos are set
separately). The page is the logo's cream, never `#ffffff`. Every other shade
(backgrounds, borders, muted text, hover, dark mode) is derived from them with
`color-mix()`, so changing a colour is one line. After changing one, re-run axe in both
schemes: each must stay ≥ 5.5:1 on its own 10% tint.

| Token | Colour | Role | On cream |
|---|---|---|---|
| `--c-paper` | `#fffbfa` cream | Page background (light), text (dark) | |
| `--c-ink` | `#1e1e1e` near-black | Page background (dark), text base (light) | 16.2:1 |
| `--c-primary` | `#785340` terracotta, darkened | Primary action, links, current page | 6.6:1 |
| `--c-neutral` | `#3d3836` taupe, darkened | Text, borders, secondary buttons | 12.2:1 |
| `--c-success` | `#17663a` green | Saved, issued | 6.8:1 |
| `--c-warning` | `#8a4f00` amber | Unsaved changes, attention | 6.4:1 |
| `--c-danger` | `#b3261e` red | Delete, errors | 6.4:1 |

The logo's own terracotta (`#bc7b5b`) is only 3.3:1 on cream, so it can't carry text or a
button; `--c-primary` is that colour darkened. The header shows `static/cladex-logo.svg`,
the horizontal logo in ink (cream in dark mode, switched inside the file).

**Type scale:** body 1rem (17 px), `--fs-small` 0.9rem, `--fs-h3` 1.15rem, `--fs-h2`
1.35rem, `--fs-h1` 1.75rem. No other sizes.

**Components:** buttons (secondary default, `.primary`, `.danger`; `a.button` for links
that look like buttons), messages (`p.success`, `p.error[role=alert]` boxed with a left
rule; field errors as red text under the field), status badges (`StatusBadge`), `td.num`
for figures, `td.actions` for row actions, `div.page-title` / `div.section-title` for a
heading with its help link, `ol.comments` for a feed of notes (the text leads; author
and date follow in a muted `--fs-small` line), `details.nav-menu` for a group of nav
links (a native disclosure; the list opens over the page, and its summary is marked when
the current page is inside it).

### How Tier 3 is checked

- **Automated** (in the preview pane, same iframe loop as axe): count visible `.primary`
  elements per screen (expect exactly one where the screen has a main action), list
  distinct computed text/background colours (all should come from the palette) and font
  sizes (only the type scale), and collect the left edges of `main`'s blocks (expect one).
- **Manual**, per screen from a screenshot, the four questions: *What's the one thing to
  do here, and does it stand out? Do the sections follow the task? Do related things sit
  together and line up? Is each kind of element styled the same as everywhere else?* Plus:
  does anything appear, disappear or move when the state changes?

## Scorecard

Status per screen. `—` is not yet audited; otherwise ✅ pass, ⚠️ open issues (see log), ❌ blocking.
Date and commit the audit in the header when refreshing.

_Palette re-check, 2026-09-30 (Cladex logo colours): axe-core 4.13, light and dark, zero
violations on login, home, quotes list, new quote, products and customers lists and forms,
users, settings, units, families, my account and help, on an empty scratch DB. Screens that
need data (builder, issued quote, status badges, messages) were not re-run; their colours
were checked by calculation only (all ≥ 5.5:1 on cream and on their 10% tints, except
danger in dark mode at 5.15:1 on its tint, still above AA)._

_Last audit: 2026-09-28, first pass of both tiers complete. axe-core 4.13 in the Chromium
preview pane, light and dark schemes, on every screen including an issued, a revised and a
revision-draft quote. Real-keyboard pass (Tab through every stop, checking order, visible
focus ring and nothing hidden) on every screen. Reflow at 320 px, and text at 200% (root
font size) at 1280 px. Heuristic review of every screen. Tier 3 visual review of every screen after the palette
and layout work (automated checks plus the four questions). Still to do: VoiceOver
([issue 10](https://github.com/gerrygoo/cladex-web/issues/10)) and the core-task
measurements. The numbers point into the issue log._

| Screen | Route | axe | Keyboard | Zoom/reflow | Screen reader | Heuristics | Visual |
|---|---|---|---|---|---|---|---|
| Login | `/login` | ✅ | ✅ | ✅ | — | ✅ | ✅ |
| Home | `/` | ✅ | ✅ | ✅ | — | ✅ | ✅ |
| Quotes list | `/cotizaciones` | ✅ | ✅ | ⚠️ 7 | — | ✅ | ✅ |
| New quote | `/cotizaciones/nueva` | ✅ | ✅ | ✅ | — | ✅ | ✅ |
| Quote builder (draft) | `/cotizaciones/{folio}` | ✅ | ✅ | ✅ | — | ⚠️ 21 | ✅ |
| Issued / revised quote | `/cotizaciones/{folio}` | ✅ | ✅ | ✅ | — | ✅ | ✅ |
| Products list | `/productos` | ✅ | ✅ | ⚠️ 7 | — | ✅ | ✅ |
| Product form | `/productos/{id}` | ✅ | ✅ | ✅ | — | ✅ | ✅ |
| Customers list | `/clientes` | ✅ | ✅ | ⚠️ 7 | — | ✅ | ✅ |
| Customer form | `/clientes/{id}` | ✅ | ✅ | ✅ | — | ✅ | ✅ |
| Users (admin) | `/usuarios` | ✅ | ✅ | ⚠️ 7 | — | ✅ | ✅ |
| Settings (admin) | `/ajustes` | ✅ | ✅ | ✅ | — | ✅ | ✅ |
| Units (admin) | `/unidades` | ✅ | ✅ | ✅ | — | ✅ | ✅ |
| My account | `/mi-cuenta` | ✅ | ✅ | ✅ | — | ✅ | ✅ |
| Help | `/ayuda` | ✅ | ✅ | ✅ | — | ✅ | ✅ |

Core tasks (the ISO 9241-11 table above): not measured yet. They need a seller doing them
while someone watches.

### Performance (Core Web Vitals)

Measured 2026-09-28. Local: every screen's HTML arrives in ≤ 10 ms and loads in ≤ 120 ms.
Production (`/login`, the only page reachable without signing in, measured from the dev
machine): TTFB 334 ms, load 948 ms, CLS 0. LCP wasn't reported by the pane; field data
from real users is the proper source. See #19.

## Issue log

One line per finding. Tier 1 findings cite the WCAG criterion; Tier 2 findings cite the
heuristic and severity. Tick when fixed, with the commit.

**Tier 1 — Compliance**

- [x] **#1 · 2.5.8 Target Size.** Buttons, inputs and selects were about 21.5 px tall; the
      column-filter toggles were 12×12 px. Now at least 28 px and 24 px.
- [x] **#2 · 4.1.2 / 1.3.1.** Quote-line quantity (and free-line description and price)
      inputs had no label. Now labelled with the line's product; "Eliminar" buttons say
      which line.
- [x] **#3 · 2.4.3 Focus Order.** Tabbing out of a quantity recalculated and dropped focus
      to `<body>`. Line controls now have stable ids, so htmx restores focus; removing a
      line moves focus to the fragment.
- [x] **#4 · 4.1.3 Status Messages.** Recalculated totals, product-picker results and list
      searches are announced through `#live-status`; flash messages have `role="status"`
      and form-level errors `role="alert"`.
- [x] **#5 · 1.3.1 / 3.3.1.** Field errors are tied to their fields (`aria-describedby`,
      `aria-invalid`) on every form, and focus starts on the first one.
- [x] **#6 · 1.3.5.** Login and Mi cuenta declare `username`, `current-password` and
      `new-password`.
- [ ] **#7 · 1.4.10 Reflow.** ([issue 9](https://github.com/gerrygoo/cladex-web/issues/9)) Fixed: the header wraps, the builder's toolbar wraps, and
      every table without column filters (quote lines, issued quote, Ajustes,
      the product's materials and conversions, Unidades) scrolls in its own box. Open: the
      four filterable lists still widen the page at 320 px. They can't simply go in a scroll box, because it would
      clip the column-filter popovers. Data tables are exempt from 1.4.10, so this is low
      priority.
- [x] **#24 · 1.4.4 Resize Text.** Buttons, fields and selects used the browser's
      ~13 px form font and didn't grow with the page's text size. They now inherit the
      page font.
- [x] **#15 · 2.4.3 / 2.4.7.** Escape closed a column-filter popover but left focus on
      its now-hidden field. Focus now returns to the funnel.

**Tier 1 — Best practice (not a WCAG failure)**

- [x] **#8** · Action columns had an empty `<th>`; now a hidden "Acciones".
- [x] **#9** · Skip link "Saltar al contenido" is the first Tab stop on every page.
- [x] **#10** · `body` has an explicit `Canvas` background, so dark mode is measurable.
- [x] **#11** · The "?" help link sits next to the `<h1>`, not inside it, and says it opens
      a new tab.
- [x] **#22** · "Nuevo cliente/producto/cotización" were `<button>`s inside `<a>` (invalid
      HTML: interactive content inside a link). Now `<a class="button">`.
- [x] **#23** · The list search boxes and the users' role selects relied on a placeholder
      or had no name at all; now they have `aria-label`s.

- [x] **#25** · Row actions ("Editar", "Eliminar", and Ajustes' "Guardar", "Retirar",
      "Hacer predeterminado") didn't say which row they act on, and Ajustes' row inputs
      were all named "Nombre" / "Margen (%)". Each now names its row. The product page's
      section help links had the same name as the page's; each now names its section.

**Tier 2 — Heuristics**

- [x] **#12 · H5 Error prevention / H3 User control · severity 3.** Unsaved lines were lost
      on leaving the builder. Now "Hay cambios sin guardar." shows while there are
      pending changes, and leaving the page asks first (`beforeunload`). Note: the Claude
      preview pane doesn't display native leave prompts. The handler was verified to
      fire, but check once in a real browser.
- [x] **#13 · H1 Visibility of system status · severity 1.** "Recalculando…" shows while a
      recalculation is in flight.
- [x] **#14 · H5 Error prevention · severity 2.** "Emitir cotización" now asks for
      confirmation, explaining that prices freeze.
- [x] **#16 · H5 Error prevention · severity 3.** The delete confirmations ("¿Eliminar
      este cliente?", products, conversions, materials, disabling a user) never
      appeared. They used `hx-confirm` on plain form posts, which htmx ignores, so one
      click deleted. Verified against the pre-fix build. Now `data-confirm`.
- [x] **#17 · H8 Aesthetic and minimalist design · severity 1.** Form fields were ~20
      characters wide and cut off descriptions and addresses; search placeholders were
      truncated. Fields now grow to 28rem.
- [x] **#26 · H4 Consistency · severity 2.** On New quote the folio-series radios, and in
      the builder the "Solo productos de la familia" checkbox, sat on their own line above
      their text (a global `input { display: block }`). Now inline.
- [x] **#27 · H5 Error prevention · severity 1.** Mi cuenta didn't say the new password
      needs 8 characters until after a failed submit. The label now says so.
- [x] **#18 · H4 Consistency / NN/g tables · severity 1.** ([issue 8](https://github.com/gerrygoo/cladex-web/issues/8)) Money and quantity columns are
      left-aligned. Right-align numbers (and use tabular figures) so amounts compare
      down a column. Fixed with the Tier 3 work (#30).
- [ ] **#19 · Performance · severity 2.** ([issue 5](https://github.com/gerrygoo/cladex-web/issues/5)) Production's nginx serves static files
      uncompressed and without cache headers, and the embedded files have no
      Last-Modified. Every page load re-downloads about 60 KB (htmx 50 KB took 567 ms on
      the measured load). Fix: gzip plus `Cache-Control` at nginx, or an ETag from the
      build SHA in the Go static handler. htmx is now `defer`, so it no longer blocks the
      first paint.
- [x] **#20 · H8 / H6 Recognition · severity 2.** ([issue 6](https://github.com/gerrygoo/cladex-web/issues/6)) In the builder, the product picker lists
      a full page of products above the lines, so the quote itself (lines and totals)
      starts below the fold. Fixed with the Tier 3 work: the lines come first (#29).
- [ ] **#21 · H1 Visibility / H2 Match · severity 2.** ([issue 7](https://github.com/gerrygoo/cladex-web/issues/7)) On a draft, "Descargar PDF" shows the
      last saved state, not what's on screen. The guide says so, but the screen doesn't.
      Say so next to the link, or disable it while there are unsaved changes.

**Tier 3 — Visual design**

- [x] **#28 · Contrast / hierarchy.** Every button was the browser's default grey, so
      "Emitir cotización" looked the same as "Eliminar" or "Buscar", and nothing marked a
      screen's main action. Now: the five-colour palette, one solid primary button per
      screen, destructive buttons outlined in red, links in the primary colour and always
      underlined, the current section marked in the nav.
- [x] **#29 · Order.** In the quote builder the product picker sat above the lines and
      pushed the quote itself below the fold
      ([issue 6](https://github.com/gerrygoo/cladex-web/issues/6)). Now: margin → lines and
      totals → save/issue → "Agregar líneas" (search, results, "Línea libre") → comments.
- [x] **#30 · Alignment.** Money was left-aligned, totals sat at the page's right edge
      instead of under the Total column, row actions were right-aligned so "Guardar" moved
      between rows in Ajustes, the "?" help sat above its section heading, and the list
      search's "Buscar" button sat lower than its box. Now: `td.num` with tabular figures,
      totals in the table's footer, left-aligned action cells, headings and help links
      centred on each other, search and button on one line.
- [x] **#31 · Repetition.** Twelve font sizes in use, three different success/error looks,
      "Editar" a link next to an "Eliminar" button, statuses as bare words. Now: a
      five-step type scale, one message style per kind, both row actions as buttons,
      status badges.
- [x] **#32 · Stable layout.** "Hay cambios sin guardar." and "Recalculando…" appeared and
      disappeared beside the save buttons; "Filtros activos" appeared above the table and
      pushed it down; adding a line slid the next product button out from under the
      cursor. Now each is a persistent status in one of its states, and the clicked
      control stays under the cursor.
- [x] **#33 · Legibility.** 16 px text, ~21 px controls, 13 px form text. Now 17 px text
      and form text, 40 px controls (32 px inside table rows).
- [x] **#34 · Hierarchy.** Quote comments were a three-column table (Fecha, Usuario,
      Comentario), so the date and author took the first two columns and as much weight
      as the note itself. Now a list: the comment text at body size, then author and date
      in a smaller, muted line beneath it. Newest first, as before; the order already
      carries the sequence.
- [x] **#35 · Layout.** The login form hugged the left edge, leaving a wide window mostly
      empty beside two fields. Now it's a column as wide as its fields, centred
      horizontally; its text stays left-aligned.
- [x] **#36 · Navigation.** The bar had ten items for an admin (seven sections, Ayuda,
      the user, Salir) and crowded a phone screen. The six supporting tables are now one
      "Catálogos" disclosure menu, so the bar has Cotizaciones, Catálogos, Ayuda and the
      user; on a phone it takes two short lines under the logo. Ajustes became
      "Configuración del sistema", so it doesn't read as personal preferences. Signing
      out moved to Mi cuenta as "Cerrar sesión", next to who you're signed in as.
- [x] **#37 · Navigation.** Cotizaciones is a menu too, with "Nueva cotización" and "Ver
      cotizaciones", so creating a quote is one path from every page. Inicio drops its
      "Nueva cotización" button and is a read-only overview with no main action.
- [x] **#38 · Minimalism.** Inicio opened with a "Cladex" heading that repeated the logo
      right above it. It's gone from view; a visually hidden "Inicio" h1 keeps the
      heading outline, and the help link sits beside "Cotizaciones por etapa".
