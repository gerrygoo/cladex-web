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

## Buscar un proyecto

1. En el menú de arriba, haz clic en **Proyectos**.
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
  equipo revisa cuando ve qué está por cerrar.
- **Vendedor**: quien creó la cotización que abrió el proyecto.
- **Abierto**: cuándo se emitió esa cotización.
- **Cotización vigente**: la más reciente del proyecto, con enlace. Si dice *"revisión
  en borrador, aún sin emitir"*, alguien la está revisando.
- **Total**: el de la cotización vigente.

Más abajo están **Seguimiento** (los botones para moverlo), **Cotizaciones** (la
vigente y las que fueron revisadas, cada una con su enlace) e **Historial**.

## Dar seguimiento a un proyecto

Todo el seguimiento se hace en la página del proyecto, en la sección **Seguimiento**.
Así el equipo sabe en qué punto está cada uno y el resumen de la
[página de inicio](README.md#la-página-de-inicio) se mantiene al día.

### Anotar la probabilidad de cierre

Mientras el proyecto es prospecto, anota qué tan cerca está de cerrarse:

1. Abre el proyecto.
2. En **Probabilidad de cierre**, elige un paso: **Inicial**, **Baja**, **Media**,
   **Alta** o **Inminente**. El porcentaje junto a cada uno es solo una referencia.
3. Escribe en **Comentario** por qué cambió (opcional pero recomendado).
4. Haz clic en **Guardar probabilidad**.

En **Historial** aparece una línea como *"Probabilidad: Baja → Alta."* con tu nota.

- Desde **Alta**, el proyecto es [[relevante para pronóstico]]. No hay que marcarlo
  aparte.
- Un proyecto nuevo empieza en **Inicial**. Actualiza la probabilidad cada vez que haya
  noticias del cliente, hacia arriba o hacia abajo.

### Pasar el proyecto a la siguiente etapa

| Para pasar a | Cuándo | Botón |
|---|---|---|
| [[O.C. recibida]] | El cliente mandó su orden de compra. | **Pasar a O.C. recibida** |
| [[en entrega]] | El pedido ya va en camino o se le entregó al cliente. | **Pasar a En entrega** |
| [[cerrado]] | Ya se entregó, se facturó, se cobró y se emitió el complemento de pago (si aplica). | **Pasar a Cerrado** |

1. Abre el proyecto.
2. Debajo de la probabilidad, escribe en **Comentario** lo que dijo el cliente y los
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

## Comentar un proyecto

El **Historial**, al final de la página, junta los comentarios de todas las
cotizaciones del proyecto, los cambios de etapa y los de probabilidad. Los más nuevos
van arriba, y cada uno dice quién lo escribió, cuándo y en qué cotización quedó.

1. Abre el proyecto.
2. Escribe en **Agregar un comentario** y haz clic en **Comentar**.

El comentario se guarda en la cotización vigente, así que también se ve en la página de
esa cotización. Cualquier usuario puede comentar. Los comentarios no se editan ni se
borran.
