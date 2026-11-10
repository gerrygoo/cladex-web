# Dashboard: Mis pendientes y Equipo (#72, #70)

**Decided 2026-11-10 (Gerry), not started.** A **Dashboard** tab with two views:
**Mis pendientes** (#72), everyone's home page, and **Equipo** (#70), admins only. Built
before facturación, entrega y cobro ([estados-del-proyecto.md](estados-del-proyecto.md)).

## Decisions

1. **Mis pendientes replaces the home page** (`GET /`). Today's Vendedores pipeline table
   (`store.SalesDashboard`) stays, as a secondary section below the pendientes, for
   everyone.
2. **Seguimiento periods are derived, not stored.** The audit log already records every
   change to a proyecto's `status`, `user_id` and `next_followup_date` with its time
   (triggers since 0016–0020), and `quote_comments` has each comment's time. That is
   enough to rebuild every overdue period, past ones included, so there is no backfill
   and no second copy that can disagree. At Cladex's scale (hundreds of proyectos, a few
   thousand audit rows) the rebuild is cheap. If it ever isn't, cache it in a table kept
   by the same code, with nothing above the store changing.
3. **Pendientes libres are a new table without audit triggers.** Their text never reaches
   `audit_log`, which admins can read. Every query is scoped to the signed-in user, and no
   admin screen lists them. We tell Emilio plainly that whoever runs the server or holds
   the backups can technically read the database.
4. **Average lateness** (#70) is the mean weekdays late of the periods **resolved** in the
   chosen range. The overdue lists and amounts always show what is overdue **now**,
   whatever the period filter says.

## Seguimiento period ("vencido")

The rule from #69, applied to history. For a proyecto and one of its follow-up days D:

- It **opens** when the proyecto is still a prospecto, still has D as its next follow-up
  day, and has no comment from the start of D (America/Mexico_City) on, at the start of
  the first **weekday** after D. A comment on a Saturday after a Friday D means it never
  opened, consistent with the avisos, which skip weekends.
- It **closes** at the first of these:
  - a comment at or after the start of D (`comentario`);
  - D replaced by another day (`nueva fecha`);
  - the proyecto stops being a prospecto, or is deleted (`cambio de etapa`).
- **Days late** n = `weekdaysAfter(D, day)`, where day is the closing day or today for an
  open period, the same count the avisos use.
- **Escalated** when n reached 2 and the period was still open at 12:00 that day. That is
  the team aviso's first step.
- **Owner:** the proyecto's `user_id` when the period opened.

This lives in one store function, e.g.
`SeguimientoPeriods(ctx, from, to string, now time.Time) ([]SeguimientoPeriod, error)`,
which returns open periods plus the ones that closed within [from, to]. Both views use
it. The avisos keep their own query for now; a test checks that they agree on who is
overdue today.

## Mis pendientes (#72)

One list mixing two kinds, sortable by priority in both directions (oldest first within a
priority), and a **Completados** panel beside it, in green.

| | Seguimientos (automatic) | Pendientes libres (manual) |
|---|---|---|
| Source | the user's open periods | table `pendientes` |
| Priority | n=1 media, n=2 alta, n≥3 muy alta; can't be changed | the user picks baja / media / alta / muy alta, editable |
| Shows | folio (link), cliente, D, days late | description, date added |
| Completed | when the period closes (it disappears from the list and shows in Completados with its reason) | the user checks it off; can reopen it from Completados |

- **Add a pendiente:** "Agregar pendiente" opens an inline row, as in the QL lines. It has
  a description and a priority (default media), and the date is today's. No limit.
- **Colours:** baja azul, media amarillo, alta naranja, muy alta rojo, on the priority
  text. They must pass contrast in both themes (`docs/UX_QUALITY.md`).
- **Completados:** the user's closed periods and completed pendientes from the last 30
  days, newest first.
- `pendientes (id, user_id → users, body, priority CHECK IN ('baja','media','alta','muy_alta'), created_at, completed_at)`, no audit triggers.

## Equipo (#70), admins only

**Filters:**
- Periodo: esta semana, este mes (default) or a date range.
- Persona.
- Probabilidad mínima, e.g. Alta 75% or more. It applies to proyectos and periods.

**People:** active users who own a prospecto or had a period in range. The development
account (`ggo`) is excluded, as in `DashboardSellers`.

**Per person:**
- active proyectos (prospectos owned now);
- overdue now;
- avisos recibidos in the range (`notifications` of kind `seguimiento_vencido` to that
  user, excluding `cancelada`);
- periods escalated in the range;
- average days late (resolved in range);
- valor and utilidad of the proyectos overdue now (before IVA, as on the home page and in
  Pronóstico);
- date of their last comment.

**Overview:**
- **Ranking** by **seguimientos a tiempo**: follow-up days falling in the range that never
  opened a period, out of all follow-up days in the range. Ties go to the lower average
  lateness.
- Team totals: valor and utilidad overdue now.
- The overdue proyectos, most days late first, each with a link.
- A link to the Notificaciones history.

## Order

1. **#72:** `SeguimientoPeriods` and its tests (history replay, weekends, each closing
   reason, escalation), the `pendientes` table, the Mis pendientes page as home, with the
   pipeline section below, and the Dashboard tab.
2. **#70:** the Equipo view on the same function.

Each one is its own commit, test-covered, with `docs/guia` updated.
