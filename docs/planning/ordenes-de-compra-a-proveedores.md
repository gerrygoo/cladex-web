# Órdenes de compra a proveedores

**Designed, nothing shipped, parked** (issue #21 is labeled requiere-diseño until it's
resumed). A product goal noted by the user on
2026-10-06; the first version's scope was decided on 2026-10-22 (see
[Decisions](#decisions-2026-10-22)).

## The goal

Cladex should track and generate its own órdenes de compra to suppliers for the
products needed to deliver a proyecto. Once a client's O.C. arrives, the app should
know what has to be bought, from whom, produce the O.C. document for each supplier,
and follow each one until the goods arrive.

The 2026-10-05 whiteboard already points at this: its note on `O.C. recibida` lists
"generación de O.C. propia a proveedores" among what happens when the client's O.C.
comes in (see [the lifecycle doc](ciclo-de-vida-cotizacion-a-cobro.md#2-etapa-de-vida-del-proyecto)).

## What exists today

- **No supplier entity.** Products belong to a familia and carry a cost
  (`cost_micros`, or materials × quantity), but nothing says who Cladex buys them from.
- **No purchase order of any kind.** The shipped `oc_emitida` stage only records, as a
  status, that the client's O.C. arrived and went to the manufacturer. The document
  itself is made outside the app.
- **Costs are MXN only**, entered by hand even when the supplier bills in USD.
- Reusable: the folio sequences, the draft → frozen document pattern and the PDF
  generation used for quotes, and the proyecto as the thing an O.C. belongs to
  ([milestone plan](hitos-proyectos-y-facturas.md), M4).

## Decisions (2026-10-22)

Cladex buys mostly from one manufacturer (the CCS wire), plus occasional suppliers for
the products on QL quotes, so the first version stays small. Each answer below matches
the open question with the same number.

1. **Suppliers**: a new Proveedores catalog (razón social, RFC, contact, payment terms,
   currency). Each product gets an optional **default supplier**. Several suppliers per
   product, each with its own cost, waits until it is needed.
2. **Scope**: one O.C. per supplier per proyecto. No consolidating across proyectos and
   no buying for stock in the first version.
3. **Lines**: at `O.C. recibida`, a button generates draft O.C.s from the proyecto's
   current quote, grouped by default supplier, priced at cost, all editable. Líneas
   libres have no catalog product, so they go to a draft whose supplier is picked by
   hand, filled in from the QL supplier quote attachments (#39).
4. **Currency**: each O.C. is MXN or USD, with the exchange rate captured by hand when it
   is issued. Product costs stay MXN and are untouched.
5. **Lifecycle**: borrador → emitida (folio and frozen PDF, like quotes) → recibida, or
   cancelada. No partial receipts at first. Receiving does **not** gate `en entrega`; the
   proyecto only shows a warning while it has O.C.s that are not received.
6. **Sending**: download the PDF and send it by hand. Emailing it from the app waits for
   outbound email (#18).
7. **Who may issue one**: any user, vendedores included, for now. Revisit with the roles
   backlog.
8. **Cost feedback**: no automatic product cost updates. The proyecto records what was
   actually paid, so it can show real margin against the quoted margin (later also in
   Pronóstico).

## Open questions (as first recorded)

Answered above; kept for the reasoning:

1. **Suppliers**: a new catalog (name, fiscal data, contact, terms)? One supplier per
   product, or several with different costs?
2. **Scope of an O.C.**: one per supplier per proyecto, or can one O.C. consolidate
   lines from several proyectos? Can Cladex buy for stock, with no proyecto?
3. **Where the lines come from**: derived from the proyecto's current quote (product
   and quantity, at cost), then edited? What about líneas libres, which have no
   catalog product?
4. **Currency**: suppliers bill in USD in some cases, and the app is MXN-only today.
5. **Lifecycle**: which states matter (draft, sent, confirmed, partially received,
   received, cancelled), and does receiving goods gate the proyecto's `en entrega`?
6. **Sending**: by email from the app (depends on outbound email, #18) or downloaded
   as a PDF and sent by hand?
7. **Who may issue one**: ties into the sysadmin / admin / ventas roles backlog.
8. **Cost feedback**: should the price on a supplier O.C. update the product's cost, or
   feed a real margin per proyecto (quoted price against what was actually paid)?
