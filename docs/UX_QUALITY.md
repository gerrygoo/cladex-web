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
- [ ] Landmarks (`<header>`, `<nav>`, `<main>`) and a skip link to `<main>` (2.4.1).
      Landmarks ✅; skip link missing (#9).
- [ ] Field errors are tied to their input with `aria-describedby` and set `aria-invalid`;
      the first invalid field is focusable from an error summary (3.3.1, 3.3.3). (#5)
- [ ] htmx swaps that change content the user didn't directly touch (quote recalculation,
      totals, list search results, flash messages) are announced through a polite live
      region (4.1.3 Status Messages). (#4)
- [ ] After an htmx swap that replaces the focused element, focus lands somewhere sensible
      and not on `<body>` (2.4.3). (#3)
- [ ] Everything works with the keyboard alone, with a visible focus ring (2.1.1, 2.4.7,
      2.4.11 Focus Not Obscured). No `outline: none` in `app.css`, so browser focus
      rings show.
- [x] Text contrast ≥ 4.5:1, UI component and focus-indicator contrast ≥ 3:1 in
      `static/app.css` (1.4.3, 1.4.11). Light mode passes axe on every screen; dark mode
      passes once `body` has a background (#10).
- [ ] Click/tap targets ≥ 24×24 CSS px, including table row actions (2.5.8). (#1)
- [ ] Usable at 200% zoom and at 320 px width without horizontal page scroll, wide tables
      excepted (1.4.4, 1.4.10). (#7)
- [ ] Inputs for the user's own data declare `autocomplete` (1.3.5). (#6)
- [ ] Destructive actions (eliminar, deshabilitar, retirar) can be confirmed or undone
      (3.3.4 applies to legal/financial data, and an issued quote is financial data).
- [ ] Money and quantities have their units in text, not only in colour or position (1.3.1,
      1.4.1).

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

_Last audit: 2026-09-28 at `d41de18`, axe-core 4.13, Chromium preview pane, light and dark
schemes. The keyboard pass and heuristic review covered only the quote builder, and there has
been no VoiceOver pass yet. The numbers point into the issue log._

| Screen | Route | axe | Keyboard | Zoom/reflow | Screen reader | Heuristics |
|---|---|---|---|---|---|---|
| Login | `/login` | ⚠️ 1 | — | — | — | — |
| Home | `/` | ⚠️ 1 | — | ⚠️ 7 | — | — |
| Quotes list | `/cotizaciones` | ⚠️ 1 | — | ⚠️ 7 | — | — |
| New quote | `/cotizaciones/nueva` | ✅ | — | ⚠️ 7 | — | — |
| Quote builder | `/cotizaciones/{folio}` | ❌ 1, 2, 8 | ⚠️ 3 | ⚠️ 7 | — | ⚠️ 12, 13, 14 |
| Products list | `/productos` | ⚠️ 1, 8 | — | ⚠️ 7 | — | — |
| Product form | `/productos/{id}` | ⚠️ 1, 8 | — | ⚠️ 7 | — | — |
| Customers list | `/clientes` | ⚠️ 1, 8 | — | ⚠️ 7 | — | — |
| Customer form | `/clientes/{id}` | ⚠️ 1 | — | ⚠️ 7 | — | — |
| Users (admin) | `/usuarios` | ⚠️ 1, 8 | — | ⚠️ 7 | — | — |
| Settings (admin) | `/ajustes` | ⚠️ 1, 8 | — | ⚠️ 7 | — | — |
| Units (admin) | `/unidades` | ⚠️ 1 | — | ⚠️ 7 | — | — |
| My account | `/mi-cuenta` | ⚠️ 1 | — | ⚠️ 7 | — | — |
| Help | `/ayuda` | ✅ | — | ⚠️ 7 | — | — |

Issues that span the whole app and don't show up in axe: #4 (no live regions), #5 (errors
not tied to fields), #9–#11.

## Issue log

One line per finding. Tier 1 findings cite the WCAG criterion; Tier 2 findings cite the
heuristic and severity. Tick when fixed, with the commit.

**Tier 1 — Compliance**

- [ ] **#1 · 2.5.8 Target Size.** Buttons, inputs and selects render about 21.5 px tall
      with almost no spacing, on every screen. The column-filter `<summary>` toggles on
      the lists are 12×12 px. Likely fixed in one place: `min-height` on form controls in
      `app.css`, plus a bigger hit area for the filter toggles.
- [ ] **#2 · 4.1.2 / 1.3.1.** The quantity inputs on quote lines have no label (axe:
      critical). Add something like `aria-label="Cantidad de {producto}"`.
- [ ] **#3 · 2.4.3 Focus Order.** In the quote builder, editing a quantity and pressing Tab
      triggers the recalculation. `#quote-lines-fragment` is replaced with `outerHTML` and
      focus falls back to `<body>`. Verified in the pane. Give the inputs stable ids, or
      restore focus after the swap.
- [ ] **#4 · 4.1.3 Status Messages.** There are no live regions anywhere. Recalculated
      totals, product search results, list search and flash messages change without
      being announced.
- [ ] **#5 · 1.3.1 / 3.3.1.** Field errors are a `<p class="error">` placed after the field.
      There's no `aria-describedby` or `aria-invalid`, so a screen reader on the field
      doesn't hear the error. Applies to every form.
- [ ] **#6 · 1.3.5 Identify Input Purpose.** Login and Mi cuenta have no `autocomplete`
      (`username`, `current-password`, `new-password`).
- [ ] **#7 · 1.4.10 Reflow.** At 320 px the header doesn't wrap and clips "Salir" on every
      page. The quote builder's search row and "Línea libre" overflow. Tables and the
      list column filters widen the whole page instead of scrolling in their own
      container.

**Tier 1 — Best practice (not a WCAG failure)**

- [ ] **#8** · The action column in tables has an empty `<th>`. Add hidden text such as
      "Acciones".
- [ ] **#9** · No skip link. 2.4.1 is met by the landmarks, but a skip link helps keyboard
      users get past the nav on every page.
- [ ] **#10** · `body` has no explicit background, so in dark mode contrast tools read the
      background as white and report false failures. Add `background: Canvas`.
- [ ] **#11** · The "?" help link sits inside each `<h1>`. That adds "Ver ayuda sobre esta
      pantalla" to every heading's accessible name, and it opens a new tab without saying
      so. Move it out of the heading and mention the new tab.

**Tier 2 — Heuristics (quote builder)**

- [ ] **#12 · H5 Error prevention / H3 User control · severity 3.** Lines and quantity
      changes are kept only in the page until "Guardar borrador". Leaving the page drops
      them with no warning. Verified: a third line added, navigate away, gone on return.
      Warn with `beforeunload` while unsaved, or autosave the draft.
- [ ] **#13 · H1 Visibility of system status · severity 1.** Recalculation shows no
      progress indicator (`hx-indicator`). Fine on the LAN, but invisible on a slow link.
- [ ] **#14 · H5 Error prevention · severity 2.** "Emitir cotización" freezes prices with
      no confirmation. It can be undone via Revisar, but only by creating a new folio.
