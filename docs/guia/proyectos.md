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
| [[O.C. recibida]] | El cliente mandó su orden de compra. |
| [[en entrega]] | El pedido ya va en camino o se le entregó al cliente. |
| [[cerrado]] | Ya se entregó, se facturó, se cobró y se emitió el complemento de pago (si aplica). |

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

También llegas a un proyecto desde la [página de inicio](README.md#la-página-de-inicio)
y desde cualquiera de sus cotizaciones: en el resumen de la cotización, **Proyecto**
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

Más abajo están **Seguimiento** (el formulario y los botones para moverlo), **Cotizaciones** (la
vigente y las que fueron revisadas, cada una con su enlace) e **Historial**.

## Dar seguimiento a un proyecto

Todo el seguimiento se hace en la página del proyecto, en la sección **Seguimiento**.
Así el equipo sabe en qué punto está cada uno y el resumen de la
[página de inicio](README.md#la-página-de-inicio) se mantiene al día.

### Anotar el seguimiento

Mientras el proyecto es prospecto, anota qué tan cerca está de cerrarse y qué sigue:

1. Abre el proyecto.
2. En **Probabilidad de cierre**, elige un paso: **Inicial**, **Baja**, **Media**,
   **Alta** o **Inminente**. El porcentaje junto a cada uno es solo una referencia.
3. En **O.C. esperada**, pon la fecha en que esperas la orden de compra del cliente.
4. En **Próximo seguimiento**, pon la fecha en que hay que volver a buscar al cliente.
5. Escribe en **Comentario** qué pasó (opcional pero recomendado).
6. Haz clic en **Guardar seguimiento**.

En **Historial** aparece una línea por cada cosa que cambió, como *"Probabilidad: Baja
→ Alta."*, *"O.C. esperada: 15/10/2026."* o *"Próximo seguimiento: 09/10/2026."*, y
debajo tu nota.

- Desde **Alta**, el proyecto es [[relevante para pronóstico]]. No hay que marcarlo
  aparte.
- Un proyecto nuevo empieza en **Inicial** y sin fechas. Actualiza el seguimiento cada
  vez que haya noticias del cliente.
- Las dos fechas son opcionales. Para quitar una, bórrala y guarda.
- Cuando llega el día del **Próximo seguimiento**, el resumen del proyecto dice *"toca
  darle seguimiento"* y la [página de inicio](README.md#la-página-de-inicio) lo cuenta
  en **Seguimientos para hoy o vencidos**. La app no manda recordatorios por correo.

### Pasar el proyecto a la siguiente etapa

| Para pasar a | Cuándo | Botón |
|---|---|---|
| [[O.C. recibida]] | El cliente mandó su orden de compra. | **Pasar a O.C. recibida** |
| [[en entrega]] | El pedido ya va en camino o se le entregó al cliente. | **Pasar a En entrega** |
| [[cerrado]] | Ya se entregó, se facturó, se cobró y se emitió el complemento de pago (si aplica). | **Pasar a Cerrado** |

1. Abre el proyecto.
2. Debajo del formulario de seguimiento, escribe en **Comentario** lo que dijo el cliente y los
   siguientes pasos (opcional pero recomendado).
3. Haz clic en el botón de la siguiente etapa.

El proyecto queda en la etapa nueva y en **Historial** aparece una línea como
*"Pasó a O.C. recibida."* con tu nota, quién la escribió y cuándo.

- Solo se avanza de una etapa a la siguiente, sin saltarse ninguna.
- Si el cliente todavía pide cambios,
  [revisa la cotización](cotizaciones.md#revisar-una-cotización-emitida) **antes** de
  pasarlo a O.C. recibida. Con la orden de compra recibida la cotización ya no se puede
  revisar: la orden responde a esa cotización.
- Si hay una revisión en borrador, en lugar del botón verás *"primero emite la
  revisión"* con un enlace a ella. Emítela y vuelve al proyecto.
- Cualquier usuario puede avanzar un proyecto. Si te equivocaste y lo pasaste de más,
  pídele a un administrador que lo regrese.
:::admin
Un administrador ve además el botón **Regresar a …** para devolver el proyecto a la
etapa anterior (queda un comentario *"Regresó a …"*). Un proyecto que regresa a
prospecto conserva la probabilidad que tenía y su cotización se puede volver a revisar.
:::

## Revisar el pronóstico

El [[pronóstico]] es la página para la junta en que el equipo repasa lo que está por
cerrar. En el menú **Proyectos**, elige **Pronóstico**.

Muestra los prospectos [[relevante para pronóstico|relevantes para pronóstico]]
(probabilidad Alta o Inminente), ordenados por **O.C. esperada**: primero la más
próxima y al final los que no tienen fecha. De cada uno verás:

- El folio (con enlace al proyecto), el cliente, el total y la probabilidad.
- **Vendedor** y **Cotización**: la vigente, cuándo se emitió y su [[vigencia]].
- **O.C. esperada**.
- **Próximo evento**: lo más cercano de hoy en adelante entre el próximo seguimiento,
  la O.C. esperada y el fin de la vigencia. Lo que ya pasó sale abajo como *"Vencido"*.
- **Última novedad**: el comentario más reciente del historial, quién lo escribió y
  cuándo.
- **Contacto**: el contacto, teléfono y correo del [cliente](clientes.md).
- **Probabilidad actualizada**: cuándo se cambió por última vez, para notar las que
  llevan tiempo sin revisarse.

Debajo de cada proyecto está su formulario de seguimiento, igual al de la página del
proyecto: cambia la probabilidad o las fechas, escribe un comentario y haz clic en
**Guardar seguimiento**. Regresas al mismo proyecto en la lista.

Al final, **Total** suma los proyectos listados y **Total ponderado** suma cada total
multiplicado por su probabilidad de cierre.

- Para subir a pronóstico un prospecto que no aparece, haz clic en **Ver todos los
  prospectos**, cámbiale la probabilidad a Alta o Inminente y guarda. **Ver solo los
  relevantes para pronóstico** regresa a la lista corta.
- Si le bajas la probabilidad a un proyecto, deja de aparecer en la lista corta.

## Marcar un proyecto como perdido

Cuando el cliente ya no va a comprar. Se puede mientras el proyecto es [[prospecto]] o
tiene la [[O.C. recibida]]; uno que ya está en entrega o cerrado no se puede perder.
Perder el proyecto es lo mismo que perder su cotización: no hay que marcar nada en la
cotización.

1. Abre el proyecto.
2. En **Perder el proyecto**, escribe el **Motivo** (obligatorio): por qué se perdió.
3. Haz clic en **Marcar como perdido** y confirma *"¿Marcar este proyecto como perdido?"*.

Qué pasa al perderlo:

- Su etapa pasa a [[perdido]] y la página muestra **Proyecto perdido** con el motivo,
  cuándo se perdió y en qué etapa estaba.
- En **Historial** queda una línea *"Se perdió."* con el motivo.
- Sale de las etapas de la [página de inicio](README.md#la-página-de-inicio) y ya no
  cuenta para el pronóstico. Ahí mismo, **Perdidos** dice cuántos hay y cuánto suman.
- Su cotización ya no se puede revisar.
- Para verlos todos, en **Proyectos** filtra la columna **Etapa** por *Perdido*.

Si el proyecto revive, pídele a un administrador que lo reabra.
:::admin
Un administrador ve en un proyecto perdido el botón **Reabrir como …**, que lo regresa
a la etapa en que estaba, con la probabilidad que tenía. Queda un comentario
*"Se reabrió como …"* y el motivo original se conserva en el historial.
:::

## Comentar un proyecto

El **Historial**, al final de la página, junta los comentarios de todas las
cotizaciones del proyecto, los cambios de etapa y los de probabilidad. Los más nuevos
van arriba, y cada uno dice quién lo escribió, cuándo y en qué cotización quedó.

1. Abre el proyecto.
2. Escribe en **Agregar un comentario** y haz clic en **Comentar**.

El comentario se guarda en la cotización vigente, así que también se ve en la página de
esa cotización. Cualquier usuario puede comentar. Los comentarios no se editan ni se
borran.
