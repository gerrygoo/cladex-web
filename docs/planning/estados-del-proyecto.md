# Estados del proyecto: facturación, entrega y cobro

**Decided 2026-11-03, refined 2026-11-10 (see [Decisions 2026-11-10](#decisions-2026-11-10)); groundwork built
2026-11-10 (see [Built 2026-11-10](#built-2026-11-10-groundwork)), then #67 Cobro and #66
Entrega the same day.** Replaces the single post-O.C. line of stages
(`oc_recibida → facturado → en_entrega → cerrado`, `internal/store/projects.go`
`ProjectFlow`) with three independent states, as Emilio's issues ask: #63 (Facturas),
#66 (Estado de facturación y de entrega) and #67 (Estado de cobro). The questions still
open are posted on those issues; the transitions below are what the issues specify.

## Why the line doesn't fit

Today a proyecto moves one step at a time and the steps are gated: nothing is
`en_entrega` without a factura, and `cerrado` needs `pagado`. Payment is a side field
(`payment_status`: sin tramitar / facturado de anticipo / pagado), and P.U.E. means paid
at the moment of the factura. Cladex's real order of events varies: goods can be
delivered before the factura, payments arrive in parts, and the supplier O.C. drives
delivery. Emilio states it outright: *Entregado* and *Pendiente de facturar* is a valid
combination.

## The model (option B)

**Stage** stays a stored column, because people decide it:
`Prospecto → En curso (O.C. recibida) → Cerrado`, plus `Perdido`.

Once the client's O.C. is received, three states hang off the proyecto. **None is
stored**: each is derived from the records, so they can't disagree with them.

| State | Values | Derived from |
|---|---|---|
| Facturación | Pendiente → Pendiente complemento de pago (P.P.D.) → Facturada | the proyecto's comprobantes: a stamped (or manually recorded) factura, and for P.P.D. its stamped complemento(s) |
| Entrega | Sin emitir O.C. → En proceso de entrega → Entregado | the confirmed supplier O.C.s (#65) and a delivery record (#66) |
| Cobro | Pendiente de pago → Pago parcial → Pagado | `SUM` of a new payments table against the proyecto's total with IVA (#67) |

```mermaid
stateDiagram-v2
    state "Facturación" as F {
        [*] --> Pendiente
        Pendiente --> Facturada: factura P.U.E. timbrada o registrada
        Pendiente --> PendienteComplemento: factura P.P.D. timbrada o registrada
        PendienteComplemento --> Facturada: complemento(s) de pago
    }
    state "Entrega" as E {
        [*] --> SinOC
        SinOC --> EnProceso: O.C. al proveedor confirmada
        EnProceso --> Entregado: marcar como entregado
    }
    state "Cobro" as C {
        [*] --> PendientePago
        PendientePago --> Parcial: pago < total
        PendientePago --> Pagado: pago = total
        Parcial --> Pagado: pagos = total
        Parcial --> PendientePago: se elimina el pago
        Pagado --> Parcial: se elimina un pago
    }
```

### Where the derivation lives: a view

SQLite generated columns can only read their own row, and these states read other
tables, so they live in a **view**, `project_states` (one row per proyecto: `facturacion`,
`entrega`, `cobro`). The list query joins it, and the existing per-column filters
(`projectFilterColumns`, plain SQL expressions) filter and sort on `ps.cobro` like any
column. One definition serves the list, the proyecto page, Pronóstico and the
Dashboard. At Cladex's scale (hundreds of proyectos) the subqueries are free, with the
foreign-key indexes in place.

If a list ever gets slow, the fallback is real columns with the same names kept by
triggers on the source tables (same transaction, indexable); nothing above the store
changes. Not worth the six-trigger surface until it's needed.

## Facturas: real stamping plus a manual mode

Decided: Facturoporti stamping (built 2026-10-20, see
[facturas-timbrado-y-envio.md §9](facturas-timbrado-y-envio.md#9-built-2026-10-20)) goes
live with real credentials. Alongside it, a **manual mode** per factura records one
issued outside the app (what exactly it stores is open question 4 on #63). Today's
typed-folio fallback is a site-wide switch for when facturación is off; manual becomes a
per-factura choice, and both kinds count the same for the Facturación state.

Método de pago (P.U.E./P.P.D.) moves from receiving the O.C. to the factura draft (#61).

## Procurement changes (#64, #65)

Supersedes decisions 1 and 4 in [ordenes-de-compra-a-proveedores.md](ordenes-de-compra-a-proveedores.md):
the default supplier is **per familia**, not per product, and there is **no exchange
rate** (an O.C.'s moneda is MXN or USD, printed in its conditions, nothing converted).

## Settled

- Option B: three derived states, stage reduced to Prospecto / En curso / Cerrado / Perdido.
- States in a `project_states` view; filters use it directly.
- Real stamping + per-factura manual mode.
- SAT codes are per SKU (#62, covering #54).
- Supplier per familia; no exchange rate.
- Deleting a proyecto stays a cascade (2026-11-05): it removes its supplier O.C.s,
  confirmed ones included, as the guide already says. No block for now. Only a
  stamped (or stamping) factura blocks the delete. Revisit if confirmed O.C.s start
  getting sent to suppliers from proyectos that later get deleted by mistake.
- Vendedores don't create suppliers inline from a QL O.C. (2026-11-05, "not yet"):
  a missing supplier goes through an admin on Proveedores.

## Open (asked 2026-11-03)

On #63: what cancelling or replacing a factura does to Facturación; whether P.P.D.
payments in parts each get a complemento (parcialidades, a contador question); whether
a P.U.E. factura records its payment automatically; what manual mode stores.

On #66: cancelling (not revising) a supplier O.C.; Entregado once or per O.C. on QL; a
deal that falls through after the O.C.; who may undo "O.C. recibida"; whether Cerrado is
automatic or a button once all three states are complete.

For Gerry: converting the proyectos already past `oc_recibida` in production
(`facturado`/`en_entrega` → En curso, `cerrado` stays, their typed factura and payment
carried into the new records). Count them before writing the migration.

## Decisions 2026-11-10

Emilio answered on 11-06 (#63, #66, #67). Gerry decided:

- **Cerrado is derived, not stored.** The `project_states` view adds `cerrado` (Facturada,
  Entregado and Pagado all hold). A correction that breaks one reopens the proyecto with no
  code. The stored stage becomes `prospecto / en_curso / perdido / cancelado`.
  "Cerrado · Ganado" is the label, filterable.
- **Cancelado** (new): admin only, with a motivo. It's blocked while there is a valid
  factura or an active supplier O.C. Reactivating it, also admin only with a motivo,
  restores the previous stage.
- **Undoing O.C. recibida:** admin only, with a motivo, only with no factura and no
  confirmed supplier O.C. The proyecto goes back to prospecto at Inminente 90%.
- **Entrega is per supplier O.C.**, shown as "1 de 2 entregadas". The comprobante can be
  added later, and an admin can undo Entregado. An admin can cancel a supplier O.C. with a
  motivo, but not once it's delivered; with no other active O.C., Entrega goes back to Sin
  emitir O.C.
- **Facturación:**
  - Parcialidades: several complementos per P.P.D. factura, each showing the saldo. What
    was built 2026-10-20 handles only one, so this extends it.
  - P.U.E. requires "ya se cobró" plus a date and records its payment.
  - Cancelling a factura puts Facturación back to Pendiente until the replacement exists.
  - Manual mode: XML required, PDF optional. It reads the UUID, date, total and RFC from
    the XML and warns on a mismatch; complementos work the same way.
- **Manual mode ships first and stays as the backup.** Real Facturoporti credentials are
  coming, but the app must keep working without the PAC.
- **Payments** (one table) link to their source: a P.U.E. factura, a complemento, or
  manual. A complemento offers to **link an existing unlinked manual payment** instead of
  creating a second one (double counting). Cancelling a factura **keeps** its payments;
  refunds are out of scope.
- **Asked Emilio (#63, 11-10):** how an anticipo paid before the factura is invoiced:
  P.P.D. for the total plus a complemento, or a factura de anticipo plus a nota de crédito
  (out of scope today).
- **Order:** after the [Dashboard](dashboard.md). Then groundwork (stage migration,
  payments, `project_states`, Cancelado, undo), #67 Cobro, #66 Entrega, #63 Facturas
  (manual), and real stamping last.

## Order (as of 2026-11-03)

1. #62 + #54 SAT codes per SKU, #64 Proveedores (in progress).
2. #65 supplier O.C.s (without cancellation until answered), #61 Recibir la O.C.
3. Once #63/#66 are answered: payments table, `project_states` view, stage migration,
   then #63's table and manual mode, #66, #67.

## Built 2026-11-10: groundwork

Migration `0041_project_states.sql` (exclusive, rebuilds `projects` and `project_files`):

- `projects.status` is `prospecto / en_curso / perdido / cancelado`; `oc_recibida`,
  `facturado`, `en_entrega` and `cerrado` became `en_curso`, and `lost_from = oc_recibida`
  became `en_curso`. New `cancel_reason`, `cancelled_from`, `cancelled_at`, in the audit
  triggers. The legacy `invoice_ref` / `invoice_date` / `payment_*` / `paid_at` columns stay.
- `project_files.kind` gains `pago` and `entrega` (the latter for #66, so it needs no
  rebuild).
- `payments` (project, amount in centavos with IVA, `paid_on`, `source`
  manual/factura/complemento, nullable `comprobante_id`, nullable `file_id` into
  `project_files`, created/deleted by/at, `delete_reason`), audited. Deleting is a soft
  delete: the row stays out of every sum and the motivo goes to the history (the codebase
  keeps its records append-only: `project_files`, `quote_comments`, `comprobante_events`).
  Every proyecto with `payment_status = pagado` got one manual payment for its current
  quote's total on its `paid_at`, recorded by whoever marked it pagado per the audit log
  (else its vendedor).
- `project_states` view: `facturacion`, `entrega`, `cobro`, `paid`, `last_paid_on`,
  `cerrado`, as described above. Every proyecto query joins it.

The flow is gone (`ProjectFlow`, `MoveProject`, the stage buttons and "Regresar a…").
Stamping moves no stage (M6.4 superseded), and the typed Factura form records the folio
without moving the stage. Admin actions with a required motivo, each a line in the
proyecto's history (`quote_comments`, as every stage change before): Cancelar proyecto,
Reactivar, Deshacer O.C. recibida.

Choices made beyond the issues (smallest sensible, recorded here):

- **Cerrado · Ganado can't happen yet** (until #66, built below). `entregado` needs #66's delivery records, so the
  view never says it; the seam is the `entrega` CASE in the view. Proyectos that were
  `en_entrega` or `cerrado` before 0041 are en curso now (QA0037 in production: facturada
  by its typed folio, pagado by its migrated payment, Entrega from its supplier O.C.). #66
  can seed their deliveries from the audit log (their last status before 0041).
- **Cancelado from prospecto too**, not only en curso: the store and the admin panel allow
  both, and Reactivar goes back to whichever it was. **Perdido is prospecto only** now: a
  sale that falls through en curso is cancelled by an admin (Emilio, #66 answer 3).
- **Cancel blocks** on a factura that may be stamped (timbrada, timbrando, incierto,
  cancelación pendiente) or a confirmed supplier O.C.; a typed `invoice_ref` doesn't block
  it (nothing can cancel it). **Undo blocks** on those plus a typed `invoice_ref`; it
  clears `oc_number`/`oc_date` (they stay in the history comment and the audit log), keeps
  the O.C. files, and deletes unstamped factura drafts (they bill the order being undone,
  and drafts can't be deleted otherwise).
- **Correcting the client's O.C.** is allowed while en curso with Facturación pendiente
  (before: while `oc_recibida`, i.e. before the factura).
- **The Proyectos list** hides Cerrado · Ganado unless the Etapa filter asks for it or a
  search is typed (looking a proyecto up by folio finds it). Perdido and Cancelado stay in
  the default list, as Perdido did. The old Pago column is now Cobro (`ps.cobro`, en curso
  only).
- **Cobro with no payments is Pendiente** even for a quote whose total is $0.
- **A typed factura with a wrong folio can't be corrected** any more (before, an admin sent
  the proyecto back to O.C. recibida). The guide says to note the correction in a comment;
  #63's manual mode replaces the typed form.

## Built 2026-11-10: #67 Cobro

`store.RecordPayment` / `DeletePayment` / `ListPayments` (`internal/store/payments.go`),
`POST /proyectos/{folio}/pagos` (the proyecto's vendedor or an admin) and
`POST /proyectos/{folio}/pagos/{id}/eliminar` (admin, motivo required). The summary shows
the Estado de cobro under Estado de entrega; the **Cobro** section, after the supplier
O.C.s, has the payments table, Total con IVA / Pagado / Saldo pendiente and "Registrar
pago" (a disclosure with Fecha, Monto, ¿Quieres subir un comprobante de pago? Sí/No). The
comprobante is a `project_files` row of kind `pago`, downloaded like the O.C.'s file.

The typed "Registrar pago completado" form, the Estado de pago badge and
`payment_status` are gone from the app (the columns stay, unread). The section is
**Facturación** now. Beyond the issue:

- **A typed or stamped factura records no payment**, P.U.E. included: #63 adds "ya se
  cobró" plus a date for P.U.E. and the complemento's automatic payment. Until then every
  payment goes through Registrar pago.
- **The amount must have at most two decimals**; `$` and thousands commas are accepted.
- **A deleted payment's comprobante stays** with the proyecto's files (project_files are
  never deleted).
- **The Cobro section also shows on a cancelled proyecto with payments** (read-only), so
  an admin can still delete one; Registrar pago is en curso only.

## Built 2026-11-10: #66 Entrega

Migration `0042_entregas.sql`:

- `deliveries` (proyecto, the O.C.'s `seq`, the `purchase_order_id` that was vigente when
  marked, `delivered_on`, nullable `file_id` into `project_files` of kind `entrega`,
  created by/at, undone by/at with `undo_reason`), audited, at most one in force per
  proyecto and seq (partial unique index).
- `purchase_orders` gains `cancel_reason`, `cancelled_by`, `cancelled_at` (status
  `cancelada`, reserved since 0034), in the audit trigger.
- `project_states.entrega`: `sin_oc` without a confirmed O.C.; `entregado` when every
  confirmed O.C. has a delivery in force for its seq; else `en_proceso`. `cerrado` reads it
  unchanged, so **Cerrado · Ganado is reachable**; the list already hides it by default and
  the Etapa filter shows it.
- Marked **exclusive** only because replacing a view is `DROP VIEW` (which
  `TestBreakingMigrationsAreExclusive` flags); the previous build would run against it.
- **Legacy deliveries:** a proyecto whose last stage before 0041 was `en_entrega` or
  `cerrado` (its newest `projects` audit row at or before 0041's `applied_at`) gets a
  delivery for each confirmed supplier O.C.: dated the México City day (UTC-6) of the audit
  row that last moved it to `en_entrega` (else to `cerrado`), by that row's actor (else its
  vendedor), no comprobante, logged as the system. A proyecto that went to `en_entrega` and
  back isn't seeded. In production that should be exactly QA0037 (cerrado, one confirmed
  O.C., facturada by its typed folio and pagado by its migrated payment), which becomes
  Cerrado · Ganado again. Nothing matches, nothing is written.

`store.MarkDelivered` / `AddDeliveryFile` / `UndoDelivery` / `CancelPurchaseOrder`
(`internal/store/deliveries.go`), under the O.C.:
`POST /proyectos/{folio}/ordenes-de-compra/{id}/entregar` and `/comprobante-entrega` (the
proyecto's vendedor or an admin), `/deshacer-entrega` and `/cancelar` (admin, motivo
required). The summary's Estado de entrega shows "En proceso de entrega · OC-…" (link to the
vigente revision) or "· 1 de 2 entregadas", and "Entregado · [fecha] · Ver comprobante".
The **Entrega** section (en curso, with at least one confirmed O.C.) has one block per
confirmed O.C. with its state, who marked it, the comprobante and the actions. History
lines: "Marcado como entregado el [fecha] por [usuario] (con comprobante): orden de compra
OC-…", "Comprobante de entrega agregado por …", "Se regresó la orden de compra OC-… a En
proceso de entrega." and "Se canceló la orden de compra OC-…", the last two with "Motivo:".

Choices made beyond the issue (smallest sensible, recorded here):

- **Delivery belongs to the O.C., keyed by its seq**, which its revisions keep. A revision
  confirmed after the delivery stays delivered (it corrects the paperwork: price, address);
  `purchase_order_id` records which revision was vigente when it was marked. If a revision
  changes what is delivered, an admin undoes the delivery.
- **Undo keeps the row** (soft, like a deleted payment) and the comprobante file; the O.C.
  can be marked again, as a new row.
- **The comprobante is added once**: no replacing it (undo and mark again if it was wrong).
- **Cancelling an O.C. is on the O.C.'s page** (next to Crear revisión), not in Entrega;
  the Entrega section holds the delivery actions only. It discards a revision of that O.C.
  still in borrador. It can't be undone. On QA/QS/QI/QALC "Generar orden de compra"
  comes back; the new O.C. takes the next seq (OC-…-2, with its own date).
- **Blockers needed no change:** Cancelar proyecto and Deshacer O.C. recibida count
  `status = 'confirmada'` only, so a cancelled O.C. is already inactive; the admin panel
  reads `ps.entrega != 'sin_oc'`, which goes back to sin_oc once every O.C. is cancelled. A
  delivered O.C. still blocks them (undo the delivery, then cancel the O.C.).
- **"Entregado · [fecha]"** uses the newest delivery day; "Ver comprobante" only with exactly
  one confirmed O.C. that has one. Marking needs the proyecto en curso; the date has no lower
  bound (not checked against the O.C.'s confirmation, since the legacy ones predate theirs).
- **The Entrega section sits between Orden de compra al proveedor and Cobro**, in the
  summary's order (facturación, entrega, cobro), rather than "al final" as the issue says.
- `static/app.js`'s Sí/No file toggle is shared: `data-comprobante-form` /
  `data-comprobante-file` on both Registrar pago and Marcar como entregado.

## Built 2026-11-14: #63 Facturas, table and manual mode

**Table** (`store.ListBilling`, `/facturas`): every proyecto en curso (Cerrado · Ganado
included) with its Estado de facturación, in Pronóstico's style; it replaces the list of
comprobantes, which still open from the proyecto or the folio fiscal. Días pendiente counts
from the O.C.'s date while Facturación is Pendiente.

**Manual mode** (migration `0044_facturas_manuales.sql`, exclusive: it rebuilds
`project_files` so `kind` gains `factura` and `complemento`): `comprobantes.origen`
(`timbrado` / `manual`), `ext_folio` (the XML's serie-folio; the app's own serie/folio stay
NULL so the portal's numbering never collides with its sequence) and `pdf_file_id`.
`store.RecordManualFactura` reads UUID, tipo I, PUE/PPD, moneda, amounts, uso and forma
from the XML and inserts a `timbrada` comprobante, so the view, blockers and screens work
unchanged. Choices beyond the issue:

- **Admin only**, as #63 says. The typed-folio form (which any user could submit) is gone;
  proyectos with a typed `invoice_ref` keep counting as Facturada.
- **Mismatches warn, they don't block**: the XML is what the SAT has. The factura page shows
  "Revisa la factura" when its total differs from the quote's or its RFC from the
  customer's; the fix is cancel + record the right one.
- **P.U.E.'s payment is what is still owed**, up to the factura's total, so an anticipo
  recorded by hand isn't counted twice; none if nothing is owed.
- **Marking cancelled** (`CancelManualFactura`): SAT motivo plus a required note, blocked
  while a complemento is in force; payments stay.
- **One factura in force per proyecto** still: a PAC draft in the works blocks recording a
  manual one (and vice versa).
- The printed PDF renders from the XML as for stamped ones; the uploaded PDF, if any, is a
  separate download.

**Complementos (parcialidades)** (`store.RecordManualComplemento`, no migration): a
manual complemento is a `pago` comprobante related to its P.P.D. factura, its `total` the
sum of the XML's `ImpPagado` for that factura's UUID, `pago_fecha` the first matching
`FechaPago`. Together they can't pass the factura's total; the view already sums them.
Each one either links an unlinked payment of the same amount (`payments.comprobante_id`
set, source left `manual`) or records a new payment, source `complemento`, refused if it
would pass the Cobro balance (the message says to link it instead). Marking a complemento
cancelled reuses `CancelManualFactura`; a factura can't be marked cancelled while it has
one in force. The Facturas table flags a Pendiente complemento proyecto whose oldest
payment without a comprobante is within 5 days of, or past, the 5th of the next month.
The PAC's "Crear comprobante de pago" (one for the whole total) is unchanged and still
only offered with no complemento in force.

**Checks for stamping (slice 4):**

- **Uso de CFDI × régimen**: `store.CFDIUseAllowed` / `CFDIUsesFor`, written from the
  c_UsoCFDI catalog's "Régimen Fiscal Receptor" column (the app ships no copy of that
  sheet; re-check against catCFDI when it changes). The draft offers only the usos valid
  for the customer's régimen and validation refuses an invalid one. Validation also
  stopped accepting an empty uso de CFDI or régimen fiscal (`ValidSATCode` treats "" as
  valid, which let both through).
- **Kilos** (`store.invoiceInKilos`): on a quote priced by weight (QG, QS), a line whose
  product's own unit code is KGM gets qty = metres × kg per metre (from the line's
  pricing snapshot, to the hundredth, as the quote shows) and unit price = importe ÷ kilos
  to the micro; the importe never changes. A line without a weight stays in MTR. Checked
  against Facturoporti's sandbox with #63's example (607.80 KGM × $203.703702 =
  $123,811.11).
- **Completing data from the draft**: the customer and product edit pages take
  `?factura=<id>` and return to it; saving a product refills the draft's empty codes
  (`RefreshDraftSATCodes`, codes already on a concepto stay).
- **Deviation from #63 point 4, kept on purpose**: the familia's SAT code still fills a
  product without its own (Gerry's earlier fallback decision), so "Faltan datos para
  facturar" lists only lines left with no code at all.
- P.P.D. → forma 99 and P.U.E. ≠ 99 were already enforced (draft form and validation).
