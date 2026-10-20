# Proyectos

Al [emitir una cotización](cotizaciones.md#emitir-la-cotización) se abre su
[[proyecto]]: el negocio al que se le da seguimiento desde que el cliente recibe la
cotización hasta que se entrega y se cobra. Lleva el folio de la cotización que lo
abrió (`QA0012`) y sigue siendo el mismo aunque la cotización se
[revise](cotizaciones.md#revisar-una-cotización-emitida) (`QA0012-R1`).

Un proyecto avanza por estas etapas:

| Etapa | Qué significa |
|---|---|
| [[prospecto]] | El cliente ya tiene la cotización y todavía no manda su orden de compra. |
| [[O.C. recibida]] | El cliente mandó su orden de compra y [se capturaron sus datos](#recibir-la-orden-de-compra-del-cliente). |
| [[facturado]] | Ya está [registrada su factura](#registrar-la-factura-y-el-pago). Sin factura no se entrega. |
| [[en entrega]] | El pedido ya va en camino o se le entregó al cliente. |
| [[cerrado]] | Ya se entregó y está pagado. Sin pago no se cierra. |

Un proyecto que no se concreta se [marca como perdido](#marcar-un-proyecto-como-perdido)
y sale de estas etapas.

## Buscar un proyecto

1. En el menú **Proyectos**, elige **Ver proyectos**.
2. Escribe el folio o el nombre del cliente en *"Buscar por folio o cliente…"*.
3. Para ordenar, haz clic en el encabezado **Proyecto**, **Cliente**, **Vendedor**,
   **Etapa**, **Probabilidad**, **Total** o **Abierto**. Otro clic invierte el orden. Sin
   ordenar, van del más reciente al más antiguo.
4. Para acotar más, [filtra por columna](README.md#filtrar-las-tablas) con el embudo de
   cada encabezado. Por ejemplo, en **Etapa** elige *Prospecto* y en **Probabilidad**
   elige *Alta* e *Inminente* para ver solo los [[relevante para pronóstico|relevantes para pronóstico]].
5. Haz clic en el folio para abrir el proyecto.

También llegas a un proyecto desde la lista de **Sin seguimiento** de la
[página de inicio](README.md#la-página-de-inicio) y desde cualquiera de sus cotizaciones: en el resumen de la cotización, **Proyecto**
tiene el enlace.

- La columna **Total** es el total de la cotización vigente del proyecto.
- La columna **Probabilidad** solo se llena mientras el proyecto es prospecto.
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

Más abajo están **Seguimiento y comentarios** (el formulario y, justo debajo, el
historial), los botones para moverlo de etapa, **Orden de compra del cliente** y
**Facturación y pago** (desde que se recibe la orden) y **Cotizaciones** (la vigente y las
que fueron revisadas, cada una con su enlace).

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
   orden de compra del cliente. Con las demás probabilidades no se muestra.
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
  en **Sin seguimiento**. La app no manda recordatorios por correo.

### Recibir la orden de compra del cliente

Cuando el cliente manda su orden de compra, el proyecto pasa de [[prospecto]] a
[[O.C. recibida]] (**Orden colocada 100%**). Para eso hay que capturar los datos de la
orden. La sección **Recibir la orden de compra** solo aparece cuando la probabilidad está
en **Inminente 90%**:

1. Abre el proyecto y, si hace falta, [sube su probabilidad](#anotar-el-seguimiento) a
   Inminente.
2. En **Recibir la orden de compra**, escribe el **No. de O.C.** del cliente.
3. Revisa la **Fecha de la O.C.** (viene con la fecha de hoy; cámbiala si la orden es de
   otro día).
4. Elige la [[forma de pago]]: **P.U.E.** (pago en una sola exhibición) o **P.P.D.**
   (pago en parcialidades o diferido).
5. En **Archivo de la O.C.**, adjunta la orden (obligatorio): un PDF, JPG o PNG de hasta
   10 MB.
6. Haz clic en **Pasar a O.C. recibida**.

El proyecto queda en O.C. recibida, con la sección **Orden de compra del cliente**
(número, fecha, forma de pago y el archivo para descargar). En el historial aparece
*"Pasó a O.C. recibida."* con los datos de la orden.

- El número, la fecha, la forma de pago y el archivo son obligatorios para pasar a O.C.
  recibida. Después, para corregir los datos, un archivo nuevo es opcional.
- Si el cliente todavía pide cambios,
  [revisa la cotización](cotizaciones.md#revisar-una-cotización-emitida) **antes** de
  recibir la orden. Con la orden recibida la cotización ya no se puede revisar: la
  orden responde a esa cotización.
- Si hay una revisión en borrador, en lugar del formulario verás *"primero emite la
  revisión"* con un enlace a ella. Emítela y vuelve al proyecto.
- ¿Te equivocaste en un dato o llegó una orden corregida? Mientras el proyecto está en
  O.C. recibida, usa **Corregir la orden de compra**, en la misma sección, y haz clic
  en **Guardar O.C.** Si adjuntas otro archivo, el anterior se conserva y el nuevo
  queda marcado como *el vigente*.
- Cuando el proyecto pasa a Facturado, los datos de la orden ya no se pueden cambiar.
- Un proyecto que ya estaba en O.C. recibida sin estos datos dice *"primero captura la
  orden de compra del cliente"*: captúralos ahí mismo para poder continuar.

### Registrar la factura y el pago

Las facturas se siguen emitiendo fuera de la app; aquí se anota su folio para que el
proyecto pueda avanzar. Hay dos reglas: **sin factura no se entrega** y **sin pago no
se cierra**. Todo se hace en la sección **Facturación y pago** del proyecto, que
aparece al recibir la orden de compra y muestra el [[estado de pago]].

Qué hay que registrar depende de la [[forma de pago]] de la orden:

**Si la orden es P.U.E.** (el cliente paga y entonces se factura):

1. Abre el proyecto, que está en [[O.C. recibida]].
2. En **Registrar pago y factura**, escribe el **Folio de la factura** y revisa la
   **Fecha de pago**.
3. Haz clic en **Registrar pago y pasar a Facturado**.

El proyecto pasa a [[facturado]] y su estado de pago queda en *Pagado*.

**Si la orden es P.P.D.** (se factura un anticipo y el pago se completa después):

1. Abre el proyecto, que está en O.C. recibida.
2. En **Registrar factura de anticipo**, escribe el **Folio de la factura de anticipo**
   y revisa la **Fecha de la factura**.
3. Haz clic en **Registrar factura y pasar a Facturado**. El proyecto pasa a Facturado
   con estado de pago *Facturado de anticipo*, y ya se puede entregar.
4. Cuando el cliente termine de pagar y se emita el comprobante de pago, vuelve al
   proyecto (esté en Facturado o En entrega). En **Registrar pago completado**, escribe
   el **Folio del comprobante de pago** y la **Fecha de pago**, y haz clic en
   **Registrar pago completado**. El estado de pago queda en *Pagado*.

- En el historial queda una línea por cada registro, con el folio y la fecha.
- Una vez facturado, los datos de la orden de compra ya no se pueden cambiar.
- En **Proyectos**, la columna **Pago** muestra el estado de pago y se puede filtrar.
- ¿Te equivocaste en un folio? Pídele a un administrador que regrese el proyecto a
  O.C. recibida y regístralo de nuevo.
:::admin
Al regresar un proyecto de Facturado a O.C. recibida se borran el folio de la factura y
el estado de pago (quedan en el historial), para registrarlos de nuevo.
:::

### Pasar el proyecto a la siguiente etapa

Después de facturar, el proyecto avanza con un botón:

| Para pasar a | Cuándo | Botón |
|---|---|---|
| [[en entrega]] | El pedido ya va en camino o se le entregó al cliente. | **Pasar a En entrega** |
| [[cerrado]] | El pedido ya se entregó completo y el estado de pago es *Pagado*. | **Pasar a Cerrado** |

1. Abre el proyecto.
2. En **Etapa**, escribe en **Comentario** lo que pasó y los siguientes pasos
   (opcional pero recomendado).
3. Haz clic en el botón de la siguiente etapa.

El proyecto queda en la etapa nueva y en el historial aparece una línea como
*"Pasó a En entrega."* con tu nota, quién la escribió y cuándo.

- Solo se avanza de una etapa a la siguiente, sin saltarse ninguna.
- Si en lugar del botón **Pasar a Cerrado** ves *"primero registra el pago
  completado"*, al proyecto le falta el pago: regístralo en **Facturación y pago**.
- Cualquier usuario puede avanzar un proyecto. Si te equivocaste y lo pasaste de más,
  pídele a un administrador que lo regrese.
:::admin
Un administrador ve además el botón **Regresar a …** para devolver el proyecto a la
etapa anterior (queda un comentario *"Regresó a …"*). Un proyecto que regresa a
prospecto conserva su probabilidad y los datos de la orden de compra, y su cotización
se puede volver a revisar.
:::

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
7. **Último comentario**: el más reciente del historial, completo (si es largo, la fila
   crece para que quepa), con quién lo escribió y cuándo debajo.

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

Cuando el cliente ya no va a comprar. Se puede mientras el proyecto es [[prospecto]] o
tiene la [[O.C. recibida]]; uno que ya está facturado, en entrega o cerrado no se puede perder.
Perder el proyecto es lo mismo que perder su cotización: no hay que marcar nada en la
cotización.

1. Abre el proyecto.
2. Si es prospecto, en **Probabilidad de cierre** elige **Perdida** y escribe el
   **Motivo** (obligatorio): por qué se perdió. Si ya tiene la O.C. recibida, la
   sección **Etapa** muestra *Orden colocada 100%* y **Perdida**; elige
   **Perdida** y escribe el motivo.
3. Haz clic en **Guardar** (o en **Marcar como perdida**) y confirma si te lo pide.

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
trae la que tenía); uno que se perdió con la O.C. recibida vuelve a O.C. recibida. Queda
un comentario *"Se reabrió como …"* y el motivo original se conserva en el historial.

## Comentar un proyecto

El historial, en **Seguimiento y comentarios**, junta los comentarios de todas las
cotizaciones del proyecto, los cambios de etapa y los de probabilidad. Los más nuevos
van arriba, y cada uno dice quién lo escribió, cuándo y en qué cotización quedó.

Hay una sola caja para escribir, arriba del historial:

- Mientras el proyecto es prospecto, es el **Comentario** del seguimiento (ver
  [Anotar el seguimiento](#anotar-el-seguimiento)).
- Después, es una caja **Comentario** con su botón **Guardar**.

Este es el único lugar donde se comenta: las cotizaciones muestran los comentarios pero no
tienen caja para escribir. El comentario se guarda en la cotización vigente, así que también
se ve en la página de esa cotización. Cualquier usuario puede comentar. Los comentarios no se editan ni se
borran.
