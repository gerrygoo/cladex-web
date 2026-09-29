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

- [ ] `<html lang="es-MX">` on every page (WCAG 3.1.1).
- [ ] Every page has a unique, descriptive `<title>` (2.4.2).
- [ ] Landmarks (`<header>`, `<nav>`, `<main>`) and a skip link to `<main>` (2.4.1).
- [ ] Field errors are tied to their input with `aria-describedby` and set `aria-invalid`;
      the first invalid field is focusable from an error summary (3.3.1, 3.3.3).
- [ ] htmx swaps that change content the user didn't directly touch (quote recalculation,
      totals, list search results, flash messages) are announced through a polite live
      region (4.1.3 Status Messages).
- [ ] After an htmx swap that replaces the focused element, focus lands somewhere sensible
      and not on `<body>` (2.4.3).
- [ ] Everything works with the keyboard alone, with a visible focus ring (2.1.1, 2.4.7,
      2.4.11 Focus Not Obscured).
- [ ] Text contrast ≥ 4.5:1, UI component and focus-indicator contrast ≥ 3:1 in
      `static/app.css` (1.4.3, 1.4.11).
- [ ] Click/tap targets ≥ 24×24 CSS px, including table row actions (2.5.8).
- [ ] Usable at 200% zoom and at 320 px width without horizontal page scroll, wide tables
      excepted (1.4.4, 1.4.10).
- [ ] Destructive actions (eliminar, deshabilitar, retirar) can be confirmed or undone
      (3.3.4 applies to legal/financial data, and an issued quote is financial data).
- [ ] Money and quantities have their units in text, not only in colour or position (1.3.1,
      1.4.1).

### How Tier 1 is checked

- **Automated:** run [axe-core](https://github.com/dequelabs/axe-core) against each screen
  in the preview browser, loaded from a CDN into the page, so there's no npm dependency.
  Axe finds roughly a third of WCAG issues, so it's a floor, not a pass.
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

_Last audit: none yet._

| Screen | Route | axe | Keyboard | Zoom/reflow | Screen reader | Heuristics |
|---|---|---|---|---|---|---|
| Login | `/login` | — | — | — | — | — |
| Home | `/` | — | — | — | — | — |
| Quotes list | `/cotizaciones` | — | — | — | — | — |
| New quote | `/cotizaciones/nueva` | — | — | — | — | — |
| Quote builder | `/cotizaciones/{folio}` | — | — | — | — | — |
| Products list | `/productos` | — | — | — | — | — |
| Product form | `/productos/{id}` | — | — | — | — | — |
| Customers list | `/clientes` | — | — | — | — | — |
| Customer form | `/clientes/{id}` | — | — | — | — | — |
| Users (admin) | `/usuarios` | — | — | — | — | — |
| Settings (admin) | `/ajustes` | — | — | — | — | — |
| Units (admin) | `/unidades` | — | — | — | — | — |
| My account | `/mi-cuenta` | — | — | — | — | — |
| Help | `/ayuda` | — | — | — | — | — |

## Issue log

One line per finding. Tier 1 findings cite the WCAG criterion; Tier 2 findings cite the
heuristic and severity. Tick when fixed, with the commit.

- [ ] _(none yet)_
