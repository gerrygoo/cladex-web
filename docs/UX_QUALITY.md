# UI/UX quality

How we judge and track the quality of Cladex's interface. Two tiers:

1. **Compliance** — web standards with pass/fail criteria. A failure is a bug.
2. **Quality** — research-backed heuristics and measurements. A finding is a judgement call,
   prioritised like any other improvement.

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
      2.4.11). Checked on the lists, the column filters and the quote builder.
- [x] Text contrast ≥ 4.5:1, UI component and focus-indicator contrast ≥ 3:1 in
      `static/app.css` (1.4.3, 1.4.11), in both colour schemes.
- [x] Click/tap targets ≥ 24×24 CSS px, including table row actions (2.5.8).
- [ ] Usable at 200% zoom and at 320 px width without horizontal page scroll, wide tables
      excepted (1.4.4, 1.4.10). Everything but the list tables fits (#7).
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

## Scorecard

Status per screen. `—` is not yet audited; otherwise ✅ pass, ⚠️ open issues (see log), ❌ blocking.
Date and commit the audit in the header when refreshing.

_Last audit: 2026-09-28, after the first round of fixes. axe-core 4.13, Chromium preview
pane, light and dark schemes, 1100 px and 320 px. Keyboard pass on the lists, column
filters, forms and quote builder. No VoiceOver pass yet ([issue 10](https://github.com/gerrygoo/cladex-web/issues/10)). The numbers point into the issue
log._

| Screen | Route | axe | Keyboard | Zoom/reflow | Screen reader | Heuristics |
|---|---|---|---|---|---|---|
| Login | `/login` | ✅ | ✅ | — | — | ✅ |
| Home | `/` | ✅ | ✅ | ✅ | — | ✅ |
| Quotes list | `/cotizaciones` | ✅ | ✅ | ⚠️ 7 | — | ⚠️ 18 |
| New quote | `/cotizaciones/nueva` | ✅ | ✅ | ✅ | — | ✅ |
| Quote builder | `/cotizaciones/{folio}` | ✅ | ✅ | ✅ | — | ⚠️ 18, 20, 21 |
| Products list | `/productos` | ✅ | ✅ | ⚠️ 7 | — | ⚠️ 18 |
| Product form | `/productos/{id}` | ✅ | ✅ | ✅ | — | ✅ |
| Customers list | `/clientes` | ✅ | ✅ | ⚠️ 7 | — | ✅ |
| Customer form | `/clientes/{id}` | ✅ | ✅ | ✅ | — | ✅ |
| Users (admin) | `/usuarios` | ✅ | ✅ | ⚠️ 7 | — | ✅ |
| Settings (admin) | `/ajustes` | ✅ | ✅ | ⚠️ 7 | — | ✅ |
| Units (admin) | `/unidades` | ✅ | ✅ | ✅ | — | ✅ |
| My account | `/mi-cuenta` | ✅ | ✅ | ✅ | — | ✅ |
| Help | `/ayuda` | ✅ | ✅ | ✅ | — | ✅ |

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
      its lines table scrolls in its own box. Open: the list tables (and Ajustes) still
      widen the page at 320 px. They can't simply go in a scroll box, because it would
      clip the column-filter popovers. Data tables are exempt from 1.4.10, so this is low
      priority.
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
- [ ] **#18 · H4 Consistency / NN/g tables · severity 1.** ([issue 8](https://github.com/gerrygoo/cladex-web/issues/8)) Money and quantity columns are
      left-aligned. Right-align numbers (and use tabular figures) so amounts compare
      down a column.
- [ ] **#19 · Performance · severity 2.** ([issue 5](https://github.com/gerrygoo/cladex-web/issues/5)) Production's nginx serves static files
      uncompressed and without cache headers, and the embedded files have no
      Last-Modified. Every page load re-downloads about 60 KB (htmx 50 KB took 567 ms on
      the measured load). Fix: gzip plus `Cache-Control` at nginx, or an ETag from the
      build SHA in the Go static handler. htmx is now `defer`, so it no longer blocks the
      first paint.
- [ ] **#20 · H8 / H6 Recognition · severity 2.** ([issue 6](https://github.com/gerrygoo/cladex-web/issues/6)) In the builder, the product picker lists
      a full page of products above the lines, so the quote itself (lines and totals)
      starts below the fold. Consider putting the lines first, or collapsing the list
      until the user searches.
- [ ] **#21 · H1 Visibility / H2 Match · severity 2.** ([issue 7](https://github.com/gerrygoo/cladex-web/issues/7)) On a draft, "Descargar PDF" shows the
      last saved state, not what's on screen. The guide says so, but the screen doesn't.
      Say so next to the link, or disable it while there are unsaved changes.
