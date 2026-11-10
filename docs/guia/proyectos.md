# Proyectos

Al [emitir una cotización](cotizaciones.md#emitir-la-cotización) se abre su
[[proyecto]]: el negocio al que se le da seguimiento desde que el cliente recibe la
cotización hasta que se entrega y se cobra. Lleva el folio de la cotización que lo
abrió (`QA0012`) y sigue siendo el mismo aunque la cotización se
[revise](cotizaciones.md#revisar-una-cotización-emitida) (`QA0012-R1`).

Un proyecto está en una de estas etapas:

| Etapa | Qué significa |
|---|---|
| [[prospecto]] | El cliente ya tiene la cotización y todavía no manda su orden de compra. |
| [[en curso]] | El cliente mandó su orden de compra (*Orden colocada 100%*) y [se capturaron sus datos](#recibir-la-orden-de-compra-del-cliente). |
| [[Cerrado]] · Ganado | Ya está facturado, entregado y pagado. No se marca a mano: llega solo. |
| [[perdido]] | El cliente no compró: se [marcó como perdido](#marcar-un-proyecto-como-perdido) mientras era prospecto. |
| [[cancelado]] | El pedido se cayó después de recibir la orden de compra: lo [canceló un administrador](#cancelar-un-proyecto-o-deshacer-la-orden-de-compra). |

Mientras está en curso, el proyecto tiene tres estados que se calculan solos con lo que se
registra en él, cada uno por su lado (un pedido puede estar entregado y pendiente de
facturar, o al revés):

| Estado | Valores | Cambia con |
|---|---|---|
| [[Estado de facturación]] | Pendiente, Pendiente complemento de pago, Facturada | la factura y, si es P.P.D., sus comprobantes de pago |
| [[Estado de entrega]] | Sin emitir O.C., En proceso de entrega, Entregado | la [orden de compra al proveedor](ordenes-de-compra.md) confirmada |
| [[Estado de cobro]] | Pendiente de pago, Pago parcial, Pagado | los pagos del cliente que se registran |

Cuando los tres están completos (*Facturada*, *Entregado* y *Pagado*), el proyecto queda
**Cerrado · Ganado**. Si después algo deja de cumplirse (por ejemplo, se cancela su factura
o se borra un pago registrado por error), regresa solo a **En curso**.

> Todavía no se puede marcar un proyecto como *Entregado*: llega en una próxima versión.
> Mientras tanto ningún proyecto llega a Cerrado · Ganado.

## Buscar un proyecto

1. En el menú **Proyectos**, elige **Ver proyectos**.
2. Escribe el folio o el nombre del cliente en *"Buscar por folio o cliente…"*.
3. Para ordenar, haz clic en el encabezado **Proyecto**, **Cliente**, **Vendedor**,
   **Etapa**, **Probabilidad**, **Próximo seguimiento**, **Total** o **Abierto**. Otro clic
   invierte el orden. Sin ordenar, van del más reciente al más antiguo.
4. Para acotar más, [filtra por columna](README.md#filtrar-las-tablas) con el embudo de
   cada encabezado. Por ejemplo, en **Etapa** elige *Prospecto* y en **Probabilidad**
   elige *Alta* e *Inminente* para ver solo los [[relevante para pronóstico|relevantes para pronóstico]].
5. Haz clic en el folio para abrir el proyecto.

También llegas a un proyecto desde las listas de **Proyectos totales** y **Sin seguimiento** de la
[página de inicio](README.md#la-página-de-inicio) y desde cualquiera de sus cotizaciones: en el resumen de la cotización, **Proyecto**
tiene el enlace.

- La columna **Total** es el total de la cotización vigente del proyecto.
- La columna **Cobro** muestra el [[estado de cobro]] de los proyectos en curso. Se puede
  filtrar con su embudo.
- Los proyectos **Cerrado · Ganado** no aparecen en la lista. Para verlos, en el embudo de
  **Etapa** elige *Cerrado · Ganado*. Si buscas uno por su folio o su cliente, sí aparece.
- La columna **Probabilidad** solo se llena mientras el proyecto es prospecto.
- La columna **Próximo seguimiento** también se llena solo mientras el proyecto es
  prospecto: la fecha que anotaste, o *Sin fecha* si no tiene. Si esa fecha ya pasó,
  dice *Vencido* debajo. Al ordenar por ella, primero van las fechas más cercanas, luego
  los prospectos sin fecha y al final los proyectos en otras etapas. Esta columna no
  tiene embudo de filtro.
- Los borradores que nunca se emitieron no tienen proyecto, así que no aparecen aquí.

## Ver el detalle de un proyecto

Debajo del folio y del cliente verás este resumen:

- **Etapa** del proyecto.
- **Probabilidad**: la [[probabilidad de cierre]] y cuándo se actualizó por última vez
  (solo mientras es prospecto). Si dice *Relevante para pronóstico*, es de los que el
  equipo revisa en el [pronóstico](#revisar-el-pronóstico).
- **O.C. esperada** y **Próximo seguimiento**: las fechas que anotaste (solo mientras
  es prospecto).
- **Vendedor**: quien creó la cotización que abrió el proyecto.
- **Abierto**: cuándo se emitió esa cotización.
- **Cotización vigente**: la más reciente del proyecto, con enlace. Si dice *"revisión
  en borrador, aún sin emitir"*, alguien la está revisando.
- **Total**: el de la cotización vigente.
- **Costo sin IVA**, **Utilidad** (precio sin IVA menos costo) y **Margen** (utilidad
  entre precio): los de la cotización vigente, igual que en el
  [pronóstico](#revisar-el-pronóstico). No aparecen mientras la vigente es una revisión en
  borrador.
- **Estado de facturación**, **Estado de entrega** y **Estado de cobro**: solo mientras el
  proyecto está en curso. Se calculan solos; ver
  [Los estados de un proyecto en curso](#los-estados-de-un-proyecto-en-curso).

Más abajo están **Seguimiento y comentarios** (la probabilidad, las fechas o, desde que se
recibe, la orden de compra, el comentario y, justo debajo, el historial), **Facturación y
pago** y **Orden de compra al proveedor** (desde que se recibe la orden; ver
[Órdenes de compra a proveedor](ordenes-de-compra.md)) y **Cotizaciones** (la vigente y las
que fueron revisadas, cada una con su enlace).

En un proyecto **QL**, antes de **Cotizaciones** está también la **Cotización del
proveedor**: el proveedor, su número de cotización y los archivos, los mismos que en la
cotización (ver [Cotización del proveedor en
QL](cotizaciones.md#cotización-del-proveedor-en-ql)). Aquí también puedes corregir el
proveedor y el número y agregar más archivos; los archivos no se eliminan desde aquí.

## Dar seguimiento a un proyecto

Todo el seguimiento se hace en la página del proyecto, en la sección **Seguimiento y
comentarios**.
Así el equipo sabe en qué punto está cada uno y el resumen de la
[página de inicio](README.md#la-página-de-inicio) se mantiene al día.

### Anotar el seguimiento

Mientras el proyecto es prospecto, anota qué tan cerca está de cerrarse y qué sigue:

1. Abre el proyecto.
2. En **Probabilidad de cierre**, elige un paso: **Inicial**, **Baja**, **Media**,
   **Alta** o **Inminente**. El porcentaje junto a cada uno es solo una referencia.
   Más a la derecha ves **Orden colocada 100%** (no se elige: el proyecto llega ahí
   solo al [recibir la orden de compra](#recibir-la-orden-de-compra-del-cliente)) y
   **Perdida** (para [marcar el proyecto como perdido](#marcar-un-proyecto-como-perdido)).
3. Si elegiste **Inminente**, aparece **O.C. esperada**: pon la fecha en que esperas la
   orden de compra del cliente. Con las demás probabilidades no se muestra. Con la
   probabilidad guardada en Inminente, justo debajo de las fechas aparece también
   [**Recibir la orden de compra**](#recibir-la-orden-de-compra-del-cliente).
4. En **Próximo seguimiento**, pon la fecha en que hay que volver a buscar al cliente.
   Es obligatoria y tiene que ser posterior a hoy.
5. Escribe en **Comentario** qué pasó y cuáles son los siguientes pasos. Es obligatorio.
6. Haz clic en **Guardar**.

Las dos fechas ya vienen con las que tiene el proyecto. Si solo quieres dejar un
comentario y el próximo seguimiento todavía no llega, escribe el comentario y guarda; si
ya llegó o pasó, pon una fecha nueva.

En el historial, justo debajo, aparece una línea por cada cosa que cambió, como *"Probabilidad: Baja
→ Alta."*, *"O.C. esperada: 15/10/2026."* o *"Próximo seguimiento: 09/10/2026."*, y
debajo tu nota.

- Desde **Alta**, el proyecto es [[relevante para pronóstico]]. No hay que marcarlo
  aparte.
- Un proyecto nuevo empieza en **Inicial** y sin fechas. Actualiza el seguimiento cada
  vez que haya noticias del cliente.
- **O.C. esperada** es opcional y solo se ve en **Inminente**. Para quitarla, bórrala y
  guarda. Si bajas la probabilidad, la fecha que tenía se conserva aunque no se vea.
- Sin comentario o sin una fecha de próximo seguimiento posterior a hoy, el seguimiento
  no se guarda, aunque no hayas cambiado la probabilidad.
- Cuando llega el día del **Próximo seguimiento**, el resumen del proyecto dice *"toca
  darle seguimiento"* y la [página de inicio](README.md#la-página-de-inicio) lo cuenta
  en **Sin seguimiento**. Si el día pasa sin un comentario nuevo, el vendedor recibe
  un [aviso por correo](acceso.md#recibir-avisos), si un administrador se lo activó.

### Recibir la orden de compra del cliente

Cuando el cliente manda su orden de compra, el proyecto pasa de [[prospecto]] a
[[en curso]] (**Orden colocada 100%**). Para eso hay que capturar los datos de la
orden. La sección **Recibir la orden de compra** solo aparece cuando la probabilidad está
en **Inminente 90%**, justo debajo de **O.C. esperada** y **Próximo seguimiento**:

1. Abre el proyecto y, si hace falta, [sube su probabilidad](#anotar-el-seguimiento) a
   Inminente.
2. En **Recibir la orden de compra**, escribe el **No. de O.C.** del cliente.
3. Revisa la **Fecha de la O.C.** (viene con la fecha de hoy; cámbiala si la orden es de
   otro día).
4. En **Archivo de la O.C.**, adjunta la orden (obligatorio): un PDF, JPG o PNG de hasta
   10 MB.
5. Haz clic en **Pasar a En curso**.

El proyecto queda **En curso**, con sus tres estados al principio: facturación *Pendiente*,
entrega *Sin emitir O.C.* y cobro *Pendiente de pago*. En **Seguimiento y comentarios**:

- La **Probabilidad de cierre** queda fija en **Orden colocada 100%**: los demás pasos se
  ven, pero ya no se pueden elegir.
- **O.C. esperada** y **Próximo seguimiento** ya no aparecen.
- En lugar de **Recibir la orden de compra** está **Orden de compra recibida**: el número,
  la fecha, el archivo (haz clic en su nombre para abrirlo o en **Descargar** para
  guardarlo) y en **Registró**, quién la capturó y cuándo.
- El **Comentario** sigue ahí para agregar notas (por ejemplo, *"Se mandó el comprobante
  de pago al proveedor"*): escríbelo y haz clic en **Guardar**. Ya no pide fecha de
  seguimiento.

En el historial aparece *"Pasó a En curso."* con los datos de la orden.

- El número, la fecha y el archivo son obligatorios para pasar a En curso. Después, para
  corregir los datos, un archivo nuevo es opcional.
- El [[método de pago]] (P.U.E. o P.P.D.) ya no se pide aquí: se elige al
  [registrar la factura](#registrar-la-factura-y-el-pago). Si el proyecto ya tenía uno
  guardado de cuando se pedía con la orden, se conserva y viene elegido al facturar.
- Si el cliente todavía pide cambios,
  [revisa la cotización](cotizaciones.md#revisar-una-cotización-emitida) **antes** de
  recibir la orden. Con la orden recibida la cotización ya no se puede revisar: la
  orden responde a esa cotización.
- Si hay una revisión en borrador, en lugar del formulario verás *"primero emite la
  revisión"* con un enlace a ella. Emítela y vuelve al proyecto.
- ¿Te equivocaste en un dato o llegó una orden corregida? Mientras el proyecto no tenga
  factura, haz clic en **Corregir la orden de compra**, debajo de **Orden de compra
  recibida**, y luego en **Guardar O.C.** Si adjuntas otro archivo, el anterior se
  conserva y el nuevo queda marcado como *el vigente*.
- Con la factura registrada, los datos de la orden ya no se pueden cambiar.
- ¿La orden se capturó en el proyecto equivocado, o no era una orden? Un administrador
  puede [deshacerla](#cancelar-un-proyecto-o-deshacer-la-orden-de-compra).
- Un proyecto que pasó a En curso antes de que se pidieran estos datos lo dice ahí mismo:
  captúralos para poder facturarlo.

### Registrar la factura y el pago

> Con la facturación configurada en el servidor, la factura y el comprobante de pago se
> timbran desde la app y el Estado de facturación cambia solo: ver [Facturas](facturas.md).
> Lo que sigue es para registrar a mano una factura hecha fuera de la app.

Las facturas se siguen emitiendo fuera de la app; aquí se anota su folio. Todo se hace en
la sección **Facturación y pago** del proyecto, que aparece al recibir la orden de compra y
muestra el [[estado de pago]].

1. Abre el proyecto, que está [[en curso]].
2. En **Registrar la factura**, elige el [[método de pago]] de la factura (obligatorio):
   **P.U.E.** (el cliente ya pagó) o **P.P.D.** (es la factura de anticipo y el pago se
   completa después). Si el proyecto ya tenía uno guardado, viene elegido; cámbialo si
   la factura dice otro.
3. Escribe el **Folio de la factura** y revisa la **Fecha de la factura**.
4. Haz clic en **Registrar factura**.

Su [[estado de facturación]] pasa a *Facturada*; la etapa no cambia. Además:

- **P.U.E.**: su estado de pago queda en *Pagado*. No hay que registrar nada más.
- **P.P.D.**: su estado de pago queda en *Facturado de anticipo*. Cuando el cliente termine
  de pagar y se emita el comprobante de pago, vuelve al proyecto. En **Registrar pago
  completado**, escribe el **Folio del comprobante de pago** y la **Fecha de pago**, y haz
  clic en **Registrar pago completado**. El estado de pago queda en *Pagado*.

Desde ese momento, **Facturación y pago** muestra el **Método de pago** y la factura
(en una P.P.D., la **Factura de anticipo**).

- En el historial queda una línea por cada registro, con el folio y la fecha.
- Una vez registrada la factura, los datos de la orden de compra ya no se pueden cambiar.
- ¿Te equivocaste en un folio? Anota la corrección en un comentario: por ahora la factura
  registrada a mano no se puede cambiar.

### Los estados de un proyecto en curso

En el resumen del proyecto en curso, debajo de **Margen**, se ven sus tres estados. Ninguno
se escoge: cada uno sale de lo que está registrado.

- **Estado de facturación**: *Pendiente* hasta que hay factura. Con una factura P.U.E.
  timbrada (o registrada a mano), *Facturada*. Con una P.P.D. timbrada, *Pendiente
  complemento de pago* hasta que sus comprobantes de pago timbrados cubren el total; luego
  *Facturada*. Si se cancela la factura, regresa a *Pendiente*.
- **Estado de entrega**: *Sin emitir O.C.* hasta que un administrador confirma la
  [orden de compra al proveedor](ordenes-de-compra.md); entonces *En proceso de entrega*,
  con el número de la orden y su enlace.
- **Estado de cobro**: *Pendiente de pago*, *Pago parcial* o *Pagado*, según los pagos
  registrados contra el total con IVA del proyecto.

## Revisar el pronóstico

El [[pronóstico]] es la página para la junta en que el equipo repasa lo que está por
cerrar. En el menú **Proyectos**, elige **Pronóstico**.

Es una tabla con un prospecto por fila que muestra los prospectos
[[relevante para pronóstico|relevantes para pronóstico]] (probabilidad Alta o Inminente).
Las columnas, en orden:

1. **No. de proyecto**: el folio, con enlace al proyecto. Queda fijo a la izquierda si la
   tabla se desliza de lado.
2. **Cliente** y **Vendedor**.
3. **Probabilidad de cierre**.
4. **Próximo seguimiento**, **O.C. esperada** y **Vigencia** de la cotización vigente.
5. **Días sin actualizar**: cuántos días hace del último movimiento del proyecto (su
   último comentario o cambio).
6. **Costo sin IVA**, **Precio de venta sin IVA**, **Utilidad** (precio menos costo) y
   **Margen** (utilidad entre precio).
7. **Último comentario**: el más reciente del historial, con quién lo escribió y cuándo
   debajo. Se ven hasta dos renglones; si es más largo, la orilla de abajo se desvanece y
   el resto se lee deslizando dentro de la celda (con la rueda del mouse, o con las
   flechas después de llegar a la celda con Tab). Quién y cuándo se ven siempre.

En una laptop se ven sin deslizar de lado las columnas desde **No. de proyecto** hasta
**Precio de venta sin IVA**; las demás, deslizando la tabla a la derecha.

La fila **Total** suma costo, precio y utilidad; su margen es la suma de las utilidades
entre la suma de los precios, no un promedio de márgenes.

Lo que necesita atención se marca con una palabra en rojo, no solo con color:

- **Próximo seguimiento**: *Vencido* si es hoy o ya pasó, y *Sin fecha* si no tiene.
- **O.C. esperada**: *Vencida* si esa fecha ya pasó.
- **Vigencia**: *Vence en N días* (de 5 días o menos), *Vence hoy* o *Vencida*.

Aquí no se edita nada. Para cambiar la probabilidad o las fechas, o para comentar, abre
el proyecto desde su número y hazlo en **Seguimiento**.

- Haz clic en el encabezado de cualquier columna para ordenar; otro clic invierte el
  orden. Sin ordenar, van por **O.C. esperada**: primero la más próxima y al final los que
  no tienen fecha.
- Para ver solo los de un vendedor, usa el embudo del encabezado **Vendedor**, como en
  [Proyectos](#buscar-un-proyecto).
- Para ver un prospecto que no aparece, haz clic en **Ver todos los prospectos**.
  **Ver solo los relevantes para pronóstico** regresa a la lista corta.
- Si le bajas la probabilidad a un proyecto, deja de aparecer en la lista corta.

## Marcar un proyecto como perdido

Cuando el cliente ya no va a comprar. Se puede mientras el proyecto es [[prospecto]]. Si el
pedido se cae después de recibir la orden de compra, el proyecto no se pierde: un
administrador lo [cancela](#cancelar-un-proyecto-o-deshacer-la-orden-de-compra).
Perder el proyecto es lo mismo que perder su cotización: no hay que marcar nada en la
cotización.

1. Abre el proyecto.
2. En **Probabilidad de cierre** elige **Perdida** y escribe el **Motivo** (obligatorio):
   por qué se perdió.
3. Haz clic en **Guardar**.

Qué pasa al perderlo:

- Su etapa pasa a [[perdido]] y la página muestra **Proyecto perdido** (la única vez que
  aparece esa sección) con el motivo,
  cuándo se perdió y en qué etapa estaba.
- En el historial queda una línea *"Se perdió."* con el motivo.
- Deja de contar en la tabla de vendedores de la [página de inicio](README.md#la-página-de-inicio)
  y en el pronóstico.
- Su cotización ya no se puede revisar.
- Para verlos todos, en **Proyectos** filtra la columna **Etapa** por *Perdido*.

Si el proyecto revive, cualquier usuario puede reabrirlo con el botón **Reabrir como …**
de la página del proyecto perdido, que lo regresa a la etapa en que estaba. Es obligatorio escribir un **Comentario**. Si estaba en
prospecto, también elige con qué **Probabilidad de cierre** vuelve (Inicial a Inminente;
trae la que tenía); uno que se perdió ya en curso, antes de que existiera *Cancelado*,
vuelve a En curso. Queda un comentario *"Se reabrió como …"* y el motivo original se
conserva en el historial.

## Cancelar un proyecto o deshacer la orden de compra

Solo un administrador, desde la sección **Administración** al final de la página del
proyecto. Las dos cosas piden un **Motivo**, que queda en el historial.

:::admin
**Cancelar proyecto.** Para un pedido que se cae (también un prospecto). Escribe el
**Motivo**, haz clic en **Cancelar proyecto** y confirma. El proyecto queda
[[cancelado|Cancelado]]: la página muestra **Proyecto cancelado** con el motivo, cuándo se
canceló y en qué etapa estaba, y en el historial queda *"Se canceló."* con el motivo. No se
puede cancelar mientras tenga una factura vigente o una orden de compra al proveedor
confirmada: primero hay que cancelar esas; la sección lo dice en lugar del botón.

**Reactivar.** En la página del proyecto cancelado, escribe el **Motivo** y haz clic en
**Reactivar como …**: regresa a la etapa en que estaba, y en el historial queda *"Se
reactivó como …"*.

**Deshacer O.C. recibida.** Para una orden de compra capturada por error (otro archivo u
otro proyecto). Escribe el **Motivo**, haz clic en **Deshacer O.C. recibida** y confirma.
El proyecto regresa a prospecto en **Inminente 90%**, sin el número ni la fecha de la orden
(quedan en el historial: *"Se deshizo la O.C. recibida (O.C. …)."*), y su cotización se
puede volver a revisar. Los archivos de la orden se conservan. No se puede si el proyecto
ya tiene factura (timbrada o registrada a mano) o una orden de compra al proveedor
confirmada.
:::

## Reasignar o eliminar un proyecto

Solo un administrador. Al final de la página del proyecto está la sección
**Administración**.

:::admin
**Reasignar.** Elige el **Vendedor responsable** y haz clic en **Reasignar**. El proyecto
y todas sus cotizaciones pasan a ese vendedor, que es quien le da seguimiento y quien
lo ve como suyo. En el historial queda *"Se reasignó de … a …"*. El PDF de sus
cotizaciones, también las ya emitidas, dice el nombre del nuevo vendedor; en la página de
la cotización, **Emitió** sigue diciendo quién la emitió y **Vendedor** dice a quién se
asignó.

**Eliminar.** Sirve, por ejemplo, para limpiar cotizaciones de prueba. Haz clic en
**Eliminar proyecto** y confirma. Se borra el proyecto con todas sus cotizaciones y
revisiones, su historial, la orden de compra, sus órdenes de compra al proveedor (también
las confirmadas) y sus facturas sin timbrar. **No se puede deshacer**, y los folios
borrados no se vuelven a usar.

Un proyecto con una factura timbrada (o en proceso de timbrarse) no se puede eliminar:
es un documento fiscal. En su lugar la página explica por qué.

Un borrador que nunca se emitió no tiene proyecto: se reasigna o se elimina desde su
propia página de [cotización](cotizaciones.md#reasignar-o-eliminar-un-borrador).
:::

## Comentar un proyecto

El historial, en **Seguimiento y comentarios**, junta los comentarios de todas las
cotizaciones del proyecto, los cambios de etapa y los de probabilidad. Los más nuevos
van arriba, y cada uno dice quién lo escribió, cuándo y en qué cotización quedó.

Hay una sola caja para escribir, arriba del historial:

- Mientras el proyecto es prospecto, es el **Comentario** del seguimiento (ver
  [Anotar el seguimiento](#anotar-el-seguimiento)).
- Después, es una caja **Comentario** con su botón **Guardar**, que no pide fecha.

Este es el único lugar donde se comenta: las cotizaciones muestran los comentarios pero no
tienen caja para escribir. El comentario se guarda en la cotización vigente, así que también
se ve en la página de esa cotización. Cualquier usuario puede comentar. Los comentarios no se editan ni se
borran.
