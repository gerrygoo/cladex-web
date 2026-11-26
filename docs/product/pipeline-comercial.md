# Track 2 · Pipeline comercial

Everything about winning the sale: who asked for prices (leads), which deals are live
(proyectos en prospecto), how likely they are (probabilidad, Pronóstico), and making sure
nobody forgets to follow up (seguimiento, avisos, Mis pendientes, Equipo).

## As built

- **Leads** (#82 L2): people who asked for prices but aren't a proyecto, with
  seguimiento, avisos, Perdido with motivo, and **Convertir en proyecto**. Price lists
  (L3) are sent from a lead and kept in its history.
- **Proyectos**: opened when a quote is issued, 1:1 with the current quote. Stage
  `prospecto → en curso → cerrado`, or `perdido` with a comment. Probabilidad de cierre
  5–90%; ≥ 75% is "relevante para pronóstico".
- **Seguimiento**: a next follow-up date per proyecto; overdue periods are derived from
  the audit log, never stored ([dashboard.md](../planning/dashboard.md)).
- **Avisos** by email, self-service per user, escalating to admins on day 2 (#69).
- **Mis pendientes** (home page) and **Equipo** (admins): overdue seguimientos plus
  personal pendientes libres.

## Signals

- **Fastest-growing track**: proyectos, avisos, dashboards and leads were all built
  between 2026-10-20 and 2026-11-25, mostly from Emilio's issues (#69–#76, #82, #87, #90).
- **Data**: 33 proyectos in five weeks; 27 still prospecto, 5 perdido, 1 en curso. The
  funnel past prospecto is almost empty so far; conversion numbers mean nothing yet.
- **Emilio cares about Pronóstico being clean**: leads exist precisely so price
  requests stop polluting it (#82). Expect more requests to keep forecast data honest.
- 93 comments on 33 proyectos: the comment thread is the team's real CRM log.

## Growth edges

| # | Edge | Trigger | Functional | Non-functional | Size | Depends on |
|---|---|---|---|---|---|---|
| V1 | **Win/loss and funnel report** | Once ~50 proyectos have left prospecto (a few months at today's rate) | Lead → proyecto → en curso → cerrado conversion by vendedor, tipo and month; motivos de pérdida; days in each stage | Derived from the audit log and stage history, like seguimiento periods. Read-only, admin. Must agree with Pronóstico's numbers (one store function, tested) | M / Internal / med | Enough data; motivos de pérdida as a closed list rather than free text (Decision) |
| V2 | **Forecast accuracy** | The weekly review uses Pronóstico to plan; nobody checks how it did | Weighted pipeline (amount × probabilidad) by expected month, and last quarter's forecast against what actually closed | Needs an expected close date on proyectos (new field), and the probabilidad at each date (already in the audit log) | M / Decision / med | V1 |
| V3 | **Customer view (cuenta)** | "What do we have going with this client?" means opening several lists | A customer page with its leads, proyectos, quotes, facturas, cobro, contacts and the comment history | Read-only joins; no new data. Contacts (V4) make it useful | S–M / Internal / high | V4 helps |
| V4 | **Contacts as their own records** | A lead is "a new contact at an existing customer" (#82 answer 6); facturas and avisos to clients need addresses per person | People belong to a customer, have email/phone/role; leads, quotes and factura recipients pick from them | Migration from today's free-text fields without losing history. Personal data of clients: who can see it, privacy notice? | M / Decision / med | — |
| V5 | **Follow up from WhatsApp** | Vendedores live in WhatsApp; an aviso there could take a reply | Reply to an aviso to log a comment or move the date | Inbound webhooks, matching a reply to a proyecto, abuse/spoofing (sender = user's verified number). Today inbound messages are only logged | L / External (Meta) / low | N4 WhatsApp (#84) |
| V6 | **Lead capture from the website or email** | Leads are typed by hand from emails and calls | A form or a mailbox that creates leads | Spam, rate limits, a public endpoint (today the app has none besides login) | M / Decision / med | Email intake (P-backlog) or a public form |
| V7 | **Activity reminders beyond seguimiento** | Avisos E (factura draft ready) and G (cobranza) are designed but waiting | Avisos for operación events, same toggles | Each aviso is a notification kind; the rounds machinery already exists | S each / Internal / high | Facturación and cobro edges (F2, F3) |

## Notes

- V1–V3 turn data already collected into decisions: the cheapest value in this track,
  limited only by how much history exists. V1 becomes meaningful around early 2027 at
  today's rate.
- V4 is the quiet prerequisite for several tracks (V3, client notifications N5, factura
  recipients, O.C. supplier contacts already exist separately). Worth doing before the
  number of customer contacts grows.
