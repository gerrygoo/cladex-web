# Glosario

Los términos que usa el cotizador. En el resto del manual, los términos subrayados con
puntos muestran su definición al pasar el cursor encima (o al tocarlos en el celular).

## Folio

Identificador único de una cotización, p. ej. `QA0012`. Se asigna al crearla y no
cambia nunca.

Una revisión conserva el folio de la original y le agrega `-R1`, `-R2`… Ver
[revisar una cotización emitida](cotizaciones.md#revisar-una-cotización-emitida).

## Serie

El prefijo del folio: **QA** (Cable CCA), **QS** (Cable CCS & AC), **QI**
(Alumbrado) o **QL** (Líneas libres); cada [[tipo de cotización]] tiene la suya. Un
administrador puede cambiarla: los folios ya dados la conservan y la numeración sigue.

Ver [¿qué serie elijo?](cotizaciones.md#crear-una-cotización).

## Familia

La línea de producto a la que pertenece cada producto del catálogo: CCA, CCS & AC o
ABASTILUM. El buscador de la cotización filtra por la familia de su
[[tipo de cotización]]. **QL** es una familia especial sin productos: solo sirve para
cotizar líneas libres. Los administradores crean familias nuevas en
[Familias y tipos de cotización](administracion.md#crear-una-familia).

## Tipo de cotización

La clase de cotización que se elige al crearla: define su [[serie]] de folio, sus
*Términos y condiciones* y cómo se ve su PDF, p. ej. *QA — Cable CCA*. No limita qué
productos lleva la cotización. Los administradores los manejan, debajo de su familia, en
[Familias y tipos de cotización](administracion.md#familias-y-tipos-de-cotización).

## SKU

La clave única de un producto en el catálogo. Es lo que escribes para buscarlo al armar
una cotización.

## Estados

Una cotización tiene tres estados:

<div class="estados">
  <span class="estado">borrador</span>
  <span class="flecha">Emitir →</span>
  <span class="estado">emitida</span>
  <span class="flecha">Revisar →</span>
  <span class="estado">revisada</span>
</div>

Al emitirla se abre su [[proyecto]], que es el que avanza por etapas:

<div class="estados">
  <span class="estado">prospecto</span>
  <span class="flecha">OC →</span>
  <span class="estado">en curso</span>
  <span class="flecha">Facturado, entregado y pagado →</span>
  <span class="estado">cerrado · ganado</span>
</div>

Un prospecto que no se concreta se marca como [[perdido]]; un proyecto en curso cuyo pedido
se cae, como [[cancelado]]. Mientras está en curso, el proyecto tiene tres estados que se
calculan solos: [[estado de facturación]], [[estado de entrega]] y [[estado de cobro]].
Cuando los tres están completos queda **Cerrado · Ganado**, sin que nadie lo marque.

La etapa se ve en la [página del proyecto](proyectos.md). Mientras el proyecto es
prospecto, si el cliente pide cambios, [Revisar](cotizaciones.md#revisar-una-cotización-emitida)
crea una nueva cotización (`QA0012-R1`) en borrador y la original pasa a **revisada**; el
proyecto sigue siendo el mismo. Cómo darle seguimiento:
[darle seguimiento](proyectos.md#dar-seguimiento-a-un-proyecto).

## Borrador

Una cotización que todavía se puede editar. Sus precios se recalculan con el catálogo,
su margen y los ajustes vigentes cada vez que la abres.

## Emitida

Una cotización congelada: sus precios, margen, términos y PDF ya no cambian.
Es la que se envía al cliente.

## Proyecto

El negocio que se abre al emitir una cotización: se le da seguimiento desde que el
cliente la recibe hasta que se entrega y se cobra. Lleva el folio de la cotización que
lo abrió (`QA0012`) y sigue siendo el mismo aunque la cotización se revise
(`QA0012-R1`, `QA0012-R2`…): su cotización vigente es siempre la más reciente.

## Prospecto

Un proyecto cuya cotización ya tiene el cliente y todavía no manda su orden de compra.
Es la primera etapa: se le da seguimiento y se anota su [[probabilidad de cierre]].

## Probabilidad de cierre

Qué tan cerca está un prospecto de convertirse en pedido. Se elige entre cinco pasos:
Inicial (10%), Baja (25%), Media (50%), Alta (75%) e Inminente (90%). Un proyecto nuevo
empieza en Inicial.

## Relevante para pronóstico

Un prospecto con probabilidad de cierre Alta o Inminente. Son los proyectos que el
equipo revisa cuando ve qué está por cerrar (lo que antes se llamaba "en pipeline"). No
se marca a mano: depende solo de la probabilidad.

## Pronóstico

La página donde el equipo repasa los prospectos relevantes para pronóstico: lo que está
por cerrar, cuándo se espera la orden de compra y cuánto suma. Está en el menú
**Proyectos**.

## O.C. recibida

El cliente ya mandó su orden de compra y el proyecto tiene su número, su fecha y su
archivo: el proyecto pasa a [[en curso]] (*Orden colocada 100%*). Desde este momento la
cotización del proyecto ya no se puede revisar.

## En curso

Un proyecto con la orden de compra del cliente recibida (*Orden colocada 100%*). Mientras
está en curso se siguen su [[estado de facturación]], su [[estado de entrega]] y su
[[estado de cobro]]; cuando los tres están completos queda [[cerrado|Cerrado · Ganado]].

## Método de pago

Cómo paga el cliente la factura. **P.U.E.**: pago en una sola exhibición (ya pagó).
**P.P.D.**: pago en parcialidades o diferido (el cliente paga después, en una o varias
partes). Se elige al registrar la factura, o en su borrador si se timbra desde
la app. Los proyectos que lo eligieron al recibir la orden de compra, cuando ahí se pedía,
lo conservan como opción inicial.

## Estado de facturación

Si un proyecto en curso ya se facturó. **Pendiente**: no tiene factura (o se canceló).
**Pendiente complemento de pago**: tiene una factura P.P.D. timbrada a la que le faltan
comprobantes de pago que cubran su total. **Facturada**: factura P.U.E. timbrada, P.P.D.
con sus comprobantes de pago, o una factura registrada a mano.

## Estado de entrega

Si ya se le pidió al proveedor y se entregó. **Sin emitir O.C.**: todavía no se confirma la
orden de compra al proveedor (o se cancelaron todas). **En proceso de entrega**: hay al
menos una confirmada que falta marcar como entregada. **Entregado**: todas las órdenes
confirmadas se marcaron como entregadas, cada una con su fecha y, si se subió, su
comprobante de entrega.

## Estado de cobro

Cuánto ha pagado el cliente, contra el total con IVA del proyecto, según los pagos
registrados en la sección **Cobro**. **Pendiente de pago**: no hay pagos registrados.
**Pago parcial**: los pagos no cubren el total. **Pagado**: lo cubren.

## Cerrado

**Cerrado · Ganado**: un proyecto en curso que ya está facturado, entregado y pagado. Llega
solo, y si algo deja de cumplirse (se cancela su factura, se borra un pago) regresa a en
curso. No aparece en la lista de proyectos salvo que la filtres por esa etapa.

## Perdido

Un prospecto que no se concretó: el cliente ya no va a comprar. Se marca con un motivo,
sale de las etapas activas y ya no cuenta para el pronóstico. Cualquier usuario
puede reabrirlo.

## Cancelado

Un proyecto cuyo pedido se cayó, casi siempre después de recibir la orden de compra. Solo
un administrador lo cancela, con un motivo, y solo si no tiene factura vigente ni orden de
compra al proveedor confirmada. También lo reactiva, y regresa a la etapa en que estaba.

## Revisada

Una cotización emitida que fue reemplazada por una revisión (`QA0012-R1`). Se conserva
intacta, con su PDF original.

## Línea libre

Una línea de cotización que no sale del catálogo (un flete, un servicio, un producto
especial). Tú escribes su descripción y su precio.

## Vigencia

Hasta cuándo es válida una cotización: el último día del mes en que se emitió. Aparece
en el PDF. En las cotizaciones QL la vigencia es una fecha que escribe el vendedor.

## IVA

16 % sobre el subtotal de la cotización.

## Material

Una materia prima con su costo por unidad, p. ej. *CCS 30%* a $160.00 por kg. Un
producto hecho de materiales (Cable CCS & AC) cuesta, por cada material, la cantidad que
lleva × su precio, y a ese costo se le aplica el margen. Los precios los captura un
administrador en Configuración del sistema.

## Margen

La parte del precio de venta que es utilidad, como porcentaje (35 %). Todo producto del
catálogo cuesta costo ÷ (1 − margen), donde el costo incluye el de sus materiales. Cada cotización usa uno
de los márgenes de una lista con nombre (p. ej. *Estándar (12.34%)*) que mantiene un
administrador en Configuración del sistema; el vendedor lo elige al armar la cotización, o escribe uno propio
(*Otro margen*), en QA, QS y QI; en QS también como precio por kilo del cobre CCS. En
QL cada línea lleva su propio margen sobre su costo.
