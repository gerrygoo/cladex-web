# Órdenes de compra a proveedor

Cuando llega la orden de compra del cliente, el proyecto arma la **orden de compra al
proveedor** con las partidas de su cotización, a costo y sin margen, en el formato
"Orden de Compra" de Cladex. Primero es un **borrador**; un administrador la
**confirma**, con lo que recibe su número y su PDF queda fijo, y la descarga para
mandarla al proveedor. La app no manda correos: el PDF se envía por fuera.

| Quién | Qué puede hacer |
|---|---|
| El vendedor del proyecto | Generar, editar, ver y eliminar el borrador, y [marcar la entrega](proyectos.md#marcar-la-entrega) de una orden confirmada |
| Un administrador | Todo lo anterior, y además confirmar y descargar, crear revisiones, cancelar una orden confirmada, regresar una entrega a En proceso y dar de alta un proveedor desde la orden (QL) |
| Cualquier otro usuario | Ver las órdenes y sus PDF |

El PDF nunca muestra el precio de venta ni el margen.

## Generar la orden de compra

1. Abre el proyecto. Desde que está **En curso** (la orden del cliente ya se
   [recibió](proyectos.md#recibir-la-orden-de-compra-del-cliente)) aparece la sección
   **Orden de compra al proveedor**.
2. Haz clic en **Generar orden de compra**. Se abre el borrador con todo precargado.

La sección lista todas las órdenes del proyecto con su **Número**, **Proveedor**,
**Estado** (*Borrador*, *Confirmada*, *Reemplazada* o *Cancelada*), **Total** y **Ver PDF**.
Cada orden confirmada aparece también en la sección **Entrega**, donde se
[marca como entregada](proyectos.md#marcar-la-entrega); su página dice su **Estado de
entrega**.

En **QA, QS, QI y QALC** el proyecto lleva una sola orden de compra: una vez generada, el
botón ya no aparece (una orden confirmada se corrige con una
[revisión](#corregir-una-orden-confirmada)). En **QL** puede llevar varias (ver
[Varias órdenes en QL](#varias-órdenes-en-ql)).

## Llenar el borrador

El borrador tiene estas partes, en orden:

- **Proveedor**: en QA, QS, QI y QALC llega el
  [proveedor predeterminado de la familia](proveedores.md) del
  proyecto; puedes cambiarlo. Si la familia no tiene uno, elígelo de la lista.
- **Partidas**: las de la cotización vigente, con la cantidad del proyecto y el **costo
  sin margen** con el que se cotizó (no el del catálogo de hoy: si el costo cambió
  después, la orden respeta el de la cotización). Ver [Cómo se calculan las
  partidas](#cómo-se-calculan-las-partidas).
- **Dirección de entrega**: el **Domicilio** (obligatorio para confirmar) y las
  **Referencias** (opcional), escritos a mano en cada orden.
- **Condiciones**: la **Moneda** y la **Forma de pago** (ver
  [Condiciones](#condiciones)).
- **Datos de facturación**: el **Uso de CFDI** con que el proveedor debe facturar esta
  compra (obligatorio para confirmar). Empieza con el predeterminado de Configuración; si
  la compra es otro tipo de gasto, elige otro (p. ej. *G03 - Gastos en general*). Los demás
  son los de Cladex (razón social, RFC, régimen fiscal y domicilio fiscal) y no se capturan
  en cada orden: un administrador los mantiene en
  [Configuración del sistema](administracion.md#datos-de-facturación-en-órdenes-de-compra).
  Una revisión empieza con el uso de CFDI de la orden que corrige.

Haz clic en **Guardar borrador**; verás *"Borrador guardado."* Los totales se actualizan
al guardar. El borrador se puede dejar incompleto y terminar después.

### Cómo se calculan las partidas

- **Productos por metro o por pieza** (QA, QI, QL): Partida · Descripción · Cantidad ·
  Precio unitario · Total. El precio unitario es el costo sin margen, al centavo (p. ej.
  CCA calibre 14 a **$5.54**), y el total es cantidad × precio unitario.
- **Productos por kilo** (QS y QALC): además, **Cantidad (m)**, **Kg por metro**, **Kg** y
  **Precio por kg** (el costo sin margen por kilo). El precio unitario por metro es precio
  por kg × kg por metro, al centavo, y el total es cantidad × ese precio. Por ejemplo,
  Alumoclad 7#8: 111 m, 0.3896 kg/m, 43.25 kg, $95.00/kg → **$37.01** por metro → total
  **$4,108.11**.
- Debajo van **Subtotal**, **IVA 16%** y **Total**. En el ejemplo de QA0037 (seis partidas
  de 200 m a $5.54, $7.77 y $12.30), subtotal $10,244.00, IVA $1,639.04 y total
  $11,883.04.

Una línea libre sin costo en la cotización dice *"Sin costo en la cotización: no se puede
comprar desde aquí."* y no se puede incluir.

### Condiciones

- **Moneda**: MXN (la predeterminada) o USD. Sale como *"Precios en pesos mexicanos (MXN),
  más IVA."* o *"Precios en dólares americanos (USD), más IVA."* No se convierte ningún
  precio.
- **Forma de pago**:
  - **Transferencia electrónica por adelantado** (la predeterminada): *"Forma de pago:
    transferencia electrónica, por adelantado conforme a la cotización de referencia."*
  - **Pago de contado**: *"Forma de pago: de contado, contra entrega."*
  - **Crédito**: aparece **Días de crédito**, con los del proveedor si los tiene en su
    ficha (si no, vacío). Es obligatorio para confirmar. Sale como *"Forma de pago:
    crédito a 30 días."*
  - **Otra**: aparece un campo de texto, obligatorio para confirmar, que sale tal cual lo
    escribas (p. ej. *"50% anticipo y 50% contra entrega."*).
- Siempre salen también *"La mercancía se recibe sujeta a revisión de cantidad y
  calidad."* y *"Favor de emitir el CFDI con los datos de facturación indicados en esta
  orden."*

## Ver el PDF

En el borrador, **Ver PDF** guarda lo que escribiste y abre en otra pestaña el PDF tal
como saldrá, con la marca **BORRADOR** y, en lugar del número, *"Se asigna al
confirmar"*. Lleva el logo de Cladex.

## Confirmar y descargar

Solo un administrador confirma la orden de compra. En el borrador, un vendedor ve *"Un
administrador confirma la orden de compra y la descarga para mandarla al proveedor."*

:::admin
1. En el borrador, revisa los datos y haz clic en **Confirmar y descargar**, y acepta
   *"¿Confirmar la orden de compra? Se le asigna su número con la fecha de hoy y ya no se
   puede editar."*
2. Verás *"Orden de compra confirmada: OC-QA0037-301026."* y se descarga el PDF final,
   sin la marca de borrador. Si la descarga no empieza sola, usa el enlace
   *"descarga el PDF aquí"*.
3. Manda el PDF al proveedor por correo, por fuera de la app.

El número es **OC-** el folio del proyecto **-** la fecha de confirmación (día, mes y año
de dos cifras, hora de la Ciudad de México): confirmada el 30/10/2026, la de QA0037 es
**OC-QA0037-301026**. La fecha del PDF es la de confirmación, y el **Vendedor** es el
responsable del proyecto.

Una orden confirmada ya no se edita. Su PDF queda fijo: aunque después cambien los datos
del proveedor o de Cladex, se vuelve a descargar igual, con **Descargar PDF** en su
página.

No se puede confirmar si falta algo; la orden se queda en borrador y el campo dice qué:

| Mensaje | Qué hacer |
|---|---|
| *Elige el proveedor.* | Elige uno de la lista. |
| *Escribe el domicilio de entrega.* | Escribe el **Domicilio** en Dirección de entrega. |
| *Elige al menos una partida.* | Marca alguna partida. |
| *Escribe los días de crédito.* | Con **Crédito**, escribe los días. |
| *Escribe la forma de pago.* | Con **Otra**, escribe el texto. |
| *Faltan datos de facturación de Cladex…* | Complétalos en [Configuración del sistema](administracion.md#datos-de-facturación-en-órdenes-de-compra). |

:::

## Corregir una orden confirmada

Una orden confirmada se corrige con una revisión, que solo crea un administrador. La
anterior queda como **Reemplazada**.

:::admin
1. Abre la orden confirmada (su número en la sección del proyecto).
2. En **Crear revisión**, escribe el **Motivo** (obligatorio, p. ej. *"Se corrigió la
   dirección de entrega"*) y haz clic en **Crear revisión**. El motivo queda en el
   historial del proyecto: *"Revisión de la orden de compra OC-QA0037-301026. Motivo: …"*
3. Se abre un borrador con los datos de la anterior. Corrige y haz clic en **Confirmar y
   descargar**.

La revisión recibe el número de la original con **-R1** (luego -R2, …), p. ej.
**OC-QA0037-301026-R1**: el número conserva la fecha original y el campo **Fecha** del PDF
dice la de la revisión. La anterior queda como **Reemplazada** y se puede seguir
consultando y descargando. Mientras la revisión es borrador, la original sigue
**Confirmada**; si eliminas el borrador de la revisión, no cambia nada.

Una revisión no cambia la entrega: si la orden ya estaba entregada, la revisión confirmada
sigue entregada.
:::

## Cancelar una orden confirmada

Para no comprarle a ese proveedor (por ejemplo, para cambiar de proveedor), un
administrador cancela la orden. No se corrige: para corregirla se usa una
[revisión](#corregir-una-orden-confirmada).

:::admin
1. Abre la orden confirmada (su número en la sección del proyecto).
2. En **Cancelar orden de compra**, escribe el **Motivo** (obligatorio) y haz clic en
   **Cancelar orden de compra**, y confirma.
3. Verás *"Orden de compra cancelada."* La orden queda como **Cancelada**, con quién la
   canceló, cuándo y el **Motivo de la cancelación**, y en el historial del proyecto queda
   *"Se canceló la orden de compra OC-…"* con el motivo.

- Una orden cancelada se sigue consultando y descargando, pero ya no cuenta: sus partidas
  quedan libres para otra orden (en QA, QS, QI y QALC vuelve a aparecer **Generar orden de
  compra**; la nueva lleva el siguiente sufijo, p. ej. **OC-QA0037-051126-2**), y si el
  proyecto no tiene otra orden confirmada su [[estado de entrega]] regresa a *Sin emitir
  O.C.*
- Una orden **entregada** no se puede cancelar: su página dice *"No se puede cancelar: ya
  está entregada."* Si se marcó por error, primero
  [regrésala a En proceso de entrega](proyectos.md#marcar-la-entrega).
- Si la orden tenía una revisión en borrador, esa revisión se elimina con ella.
- No se puede deshacer.
:::

## Varias órdenes en QL

En un proyecto **QL** cada orden es para un proveedor, con las partidas que le tocan:

- Si en la cotización se capturó el proveedor en la
  [Cotización del proveedor](cotizaciones.md#cotización-del-proveedor-en-ql) y coincide
  con uno del catálogo (por nombre comercial o razón social), llega preseleccionado.
- En **Partidas**, marca las que van en esta orden. Una partida que ya está en otra orden
  del proyecto dice *"Ya está en OC-…"* (o *"Ya está en un borrador."*) y llega sin
  marcar.
- Para otro proveedor, haz clic en **Generar otra orden de compra**: trae las partidas que
  ninguna otra orden tiene. A partir de la segunda, el número lleva un sufijo:
  **OC-QL0010-301026-2**, **-3**, etc.

:::admin
Si el proveedor no está en el catálogo, un administrador lo da de alta sin salir de la
orden: abre **Dar de alta un proveedor nuevo**, llena **Nombre comercial**, **RFC**,
**Razón social** y la dirección fiscal (y, si quieres, **Régimen fiscal** y **Días de
crédito**) y haz clic en **Dar de alta y elegir**. Se guarda el borrador, se crea el
proveedor y queda elegido: *"Proveedor dado de alta y elegido."* Sus contactos y datos
bancarios se completan después en [Proveedores](proveedores.md). Si se parece a uno que
ya existe, la app lo avisa; elige el existente o marca *"Es un proveedor distinto: darlo
de alta de todos modos"*.
:::

Un vendedor ve *"Si el proveedor no está en la lista, pide a un administrador que lo dé
de alta."*

## Eliminar un borrador

En el borrador, haz clic en **Eliminar borrador** y confirma. Solo se eliminan
borradores; una orden confirmada se queda.

## Proveedores con órdenes de compra

Un proveedor con órdenes de compra (en cualquier estado) ya no se puede eliminar del
catálogo; un administrador puede marcarlo como inactivo (ver [Proveedores](proveedores.md)).
