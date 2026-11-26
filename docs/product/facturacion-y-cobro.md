# Track 4 · Facturación y cobro

Billing the client (CFDI facturas through a PAC) and recording that they paid. The track
with legal weight: a stamped factura exists for the SAT whether or not our app heard
back.

## As built

- **Stamping through Facturoporti**, end to end against the sandbox (2026-10-20):
  borrador → lista → timbrando → timbrada, with `timbrado incierto` resolved by resending
  the identical request; sustitución (relación 04) and cancellation; PDF rendered from
  the stored XML. Admin-only. Environment guard so a sandbox stamp can't pass for a real
  one. ([facturas-timbrado-y-envio.md](../planning/facturas-timbrado-y-envio.md) §9)
- **Manual mode**: record a factura issued outside the app by uploading its XML;
  complementos de pago from their XML, in parcialidades.
- **Estado de facturación** derived from comprobantes; **Cobro** section with payments
  (forma de pago required), Estado de cobro derived; issue a complemento from a payment
  row (#83).
- Not built: standalone facturas, notas de crédito, resend by email, stamping on jobs.

## Signals

- **Production stamping is on since 2026-11-14** (real CSD and token on the NAS; #20
  closed 2026-11-25; #78, rotating the token, still open). Staging stamps against the
  sandbox. But production holds **0 comprobantes** and 1 payment: only one proyecto has
  reached en curso, so nothing has needed a factura yet.
- 38 commit subjects in this track: a lot of investment, not yet exercised by real use.
  The first real factura is the event that validates it.
- Emilio's issues (#63, #67, #83, #87) keep asking for the operational view (what's
  pending to invoice, what's pending to collect), not for more fiscal features.

## Growth edges

| # | Edge | Trigger | Functional | Non-functional | Size | Depends on |
|---|---|---|---|---|---|---|
| F1 | **First real factura, watched** | The first proyecto that needs one (one en curso today) | The first real factura is stamped, checked by the contador, sent; anything surprising becomes an issue | Rotate the token (#78) before it. Someone checks the stamped XML against the contador's expectations before the client gets it | XS / Expert (contador) / med | A proyecto ready to invoice |
| F2 | **Cuentas por cobrar** | Once a few facturas are out: "who owes us, and since when?" | A list of facturas with balance, due date (from payment terms) and age buckets (0–30, 31–60, 60+); aviso G (cobranza) to the responsible person | Due date is new data (on the factura or from customer terms). Derived balances from payments, like Estado de cobro | M / Decision / med | Real facturas (F1) |
| F3 | **Several facturas per proyecto** (anticipo + final, partial deliveries) | A real proyecto needs an anticipo invoiced first | Issue more than one factura per proyecto; Estado de facturación sums them against the total | Today's rule is one factura per proyecto (§9). The derived state, the cobro sums and the proyecto's gate all change. CFDI relación 07 for anticipos: contador | L / Expert / low | F1; contador's guidance |
| F4 | **Resend and copies by email** | Clients ask for the XML again, or to copy their accounts payable | "Reenviar" with recipients, the history shows every send and bounces | Attachments on notifications (N5). Stamped XML is immutable; the email is not | S / Internal / high | N5 |
| F5 | **Stamping as a job, with retry** | A PAC timeout leaves a factura `incierto` and an admin has to come back | The retry happens in the background; the admin sees the result | Idempotency already holds (identical bytes). Lease shorter than the PAC's timeout | S / Internal / high | Jobs (built) |
| F6 | **Notas de crédito and standalone facturas** | A return, a discount after invoicing, or a sale without a proyecto | Issue an egreso related to a factura; issue a factura with no proyecto | New comprobante tipos through the same machinery; reports must not double count | M each / Expert / low | F1 |
| F7 | **Contador export** | Month end: the contador asks for everything | Download a month's XML and PDFs as a zip, plus a CSV summary | Read-only; must include cancelled ones and their acuses | S / Internal / high | F1 |
| F8 | **Bank reconciliation** | Payments are typed by hand from the bank statement | Import a statement (CSV/OFX), match deposits to facturas, confirm | Each bank's format; matching rules; never auto-record without confirmation | L / External / low | F2 |

## Non-functional notes for the whole track

- **Retention**: stamped XML is kept for the period the SAT requires (5 years under the
  Código Fiscal; confirm with the contador). Today it lives in SQLite and therefore in
  every backup; it's tiny, so that's fine, but the offsite copy must exist (see
  [baseline](baseline.md#storage)).
- **No stamping from two places at once**: blue/green deploys run two builds briefly;
  stamping must stay safe if both are up (the runner already claims only the job kinds
  it handles; keep F5 under that rule).
- **Roles**: stamping and cancelling are admin-only by decision until roles change (P1).
