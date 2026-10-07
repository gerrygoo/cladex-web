# Órdenes de compra a proveedores

**Proposed, nothing shipped, not scheduled.** A product goal recorded so it is tracked,
not a design. Noted by the user on 2026-10-06.

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

## Open questions

To answer when this is picked up, none decided:

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
