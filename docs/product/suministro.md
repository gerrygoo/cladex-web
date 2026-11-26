# Track 3 · Suministro

After the client's O.C. arrives: buying what the proyecto needs from suppliers and
getting it delivered. The track that turns Cladex's app from a sales tool into an
operations tool.

## As built

- **Proveedores** catalog (#64), with a default supplier **per familia**; bank details
  admin-only.
- **Supplier O.C.** (#65): generated from the proyecto's current quote, grouped by
  supplier, priced at cost, preview PDF, confirmed by an admin (folio, frozen PDF),
  revisions. Uso de CFDI per O.C. Sent by hand (download the PDF).
- **Entrega** (#66): mark each supplier O.C. delivered, with an optional receipt; the
  proyecto's Estado de entrega is derived (Sin emitir O.C. → En proceso → Entregado).
- MXN only, no exchange rate; no partial receipts; no stock.
  ([ordenes-de-compra-a-proveedores.md](../planning/ordenes-de-compra-a-proveedores.md),
  [estados-del-proyecto.md](../planning/estados-del-proyecto.md))

## Signals

- **One** supplier O.C. and **one** supplier in production. The design was kept small on
  purpose ("Cladex buys mostly from one manufacturer"); real use will say what's missing.
- Proyectos reach en curso slowly (1 of 33 so far), so this track's feedback loop is
  months long. Plan to revisit it after ~10 proyectos have gone through entrega.

## Growth edges

| # | Edge | Trigger | Functional | Non-functional | Size | Depends on |
|---|---|---|---|---|---|---|
| S1 | **Real margin per proyecto** | The quoted margin and what was actually paid differ, and nobody sees it | The proyecto shows quoted cost vs O.C. cost vs quoted price: real profit and margin; Equipo totals it per month | Derived from confirmed O.C.s, never stored. Revisions of an O.C. count once (current one). Admin-only, like costs today | S–M / Internal / high | — (decision 8 of the O.C. doc already asks for it) |
| S2 | **Send the O.C. to the supplier** | Admins download and email each PDF by hand | "Enviar" emails the O.C. to the supplier's contact; history shows it | Same attachment plumbing as C1 and F4 | S / Internal / high | N5 attachments |
| S3 | **Partial deliveries** | A supplier ships in parts, and Entregado is all-or-nothing | Record what arrived per line; Estado de entrega shows Parcial | The derived state reads line-level receipts; an O.C. revision after a partial receipt must not lose it | M / Internal / med | Real cases first |
| S4 | **Delivery dates and lateness** | Clients ask "when?", and promises depend on each supplier's lead time | Each O.C. has a promised date; late ones appear on Mis pendientes and Equipo like overdue seguimientos | Reuses the derived-period approach of seguimiento; an aviso kind | M / Internal / med | — |
| S5 | **Several suppliers per product, with their costs** | A second supplier for the same products appears | Pick the supplier per line; the cost follows | Changes the cost model under quotes; issued quotes keep their snapshot cost | M / Decision / med | — |
| S6 | **USD supplier costs** | A supplier bills in USD | O.C. in USD with a captured rate; S1 converts | See C6; decide once for both | M / Decision / low | C6 |
| S7 | **Stock / inventory** | Cladex starts buying for stock or consolidating across proyectos | Warehouse quantities, reservations per proyecto | Changes the product's shape: costing method, physical counts. Explicitly out of the first version | XL / Decision / low | — |

## Notes

- S1 is the standout: small, uses only data the app already has, and answers the
  question margins exist for. It is also the bridge back into Cotizaciones (see
  [entre-tracks](entre-tracks.md#the-money-loop)).
- S3–S6 should wait for real proyectos to go through operación; designing them now would
  be guessing.
