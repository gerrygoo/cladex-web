# Cotizaciones

El flujo completo, de principio a fin:

1. [Crear la cotización](#crear-una-cotización): eliges cliente y [[serie]]; queda en
   [[borrador]] con su [[folio]].
2. Agregar líneas: [productos del catálogo](#agregar-productos-por-sku) y/o
   [líneas libres](#agregar-una-línea-libre).
3. [Ajustar cantidades](#cambiar-la-cantidad-de-una-línea) y
   [elegir el margen](#elegir-el-margen); el sistema calcula precios, [[IVA]] y total.
4. [Guardar el borrador](#guardar-el-borrador), cuantas veces quieras.
5. [Emitir](#emitir-la-cotización): se congela y se genera el PDF definitivo.
6. [Descargar el PDF](#descargar-y-enviar-el-pdf) y enviarlo al cliente.
7. ¿El cliente pidió cambios? [Revisar](#revisar-una-cotización-emitida): se crea una
   nueva versión (`QA0012-R1`).

## Crear una cotización

**Antes:** el cliente tiene que existir. Si no está, [dalo de alta](clientes.md#dar-de-alta-un-cliente).

1. En el menú, entra a **Cotizaciones** y haz clic en **Nueva cotización**.
2. En **Cliente**, elige al cliente de la lista (está en orden alfabético).
3. En **Serie de folio**, elige una:
   - **QA — Cable CCA**
   - **QS — Cable CCS & AC**
   - **QI — Alumbrado**
4. Haz clic en **Crear cotización**.

Llegas a la página de la cotización (p. ej. *"Cotización QS0007"*) con
**Estado: borrador**. El folio ya quedó reservado.

> **¿Qué serie elijo?** La [[serie]] hace dos cosas: define el folio (cada serie lleva su
> propia numeración: QA0001, QA0002…) y el bloque de **Términos y condiciones** que se
> imprime en el PDF (moneda, tiempo de entrega, empaque, flete). Elige la serie cuyos
> términos correspondan a lo que vendes. La serie **no** limita los productos: una
> cotización QS puede llevar productos de cualquier familia.

> El cliente y la serie no se pueden cambiar después, y los borradores no se pueden
> borrar. Si te equivocaste, crea otra cotización y deja la equivocada sin emitir.

## Agregar productos por SKU

Con la cotización en borrador:

1. Debajo del buscador ya aparece la lista de productos de la familia de la serie, en
   orden de SKU, 10 por página, cada uno como botón:
   `SKU — Descripción (precio)`. No hace falta escribir nada para verlos.
   - Usa **« Anterior** y **Siguiente »** para moverte entre páginas; la línea
     *"Página 1 de 3 · 23 productos"* te dice cuántos hay.
   - Para acotar la lista, escribe el SKU o parte de la descripción (p. ej. `THW`) en
     el cuadro *"Buscar producto por SKU o descripción…"*. La lista se filtra
     mientras escribes y vuelve a la página 1.
   - Junto al buscador está la casilla *"Solo productos de la familia …"* (CCA para
     serie QA, CCS & AC para QS, ABASTILUM para QI), **marcada de inicio**: solo
     aparecen productos de la familia de la serie. Desmárcala para ver o buscar en todo
     el catálogo; cualquier producto se puede agregar a cualquier cotización.
2. Haz clic en el producto. Se agrega como una línea nueva, con cantidad 1 y su precio
   ya calculado. La lista se queda en la misma página.
3. Repite para cada producto.

> El precio de una línea **lo calcula el sistema** a partir del catálogo, el
> [margen de la cotización](#elegir-el-margen) y los ajustes vigentes
> (precios de los [[material|materiales]]). No se captura a mano. Ver
> [cómo se calcula el precio de un producto](productos.md#cómo-se-calcula-el-precio-de-un-producto).
> El precio que aparece entre paréntesis en los resultados es el dato del catálogo, no
> necesariamente el precio final de la línea.

## Cambiar la cantidad de una línea

1. En la columna **Cantidad**, escribe la cantidad. Acepta decimales (hasta 3, p. ej.
   `152.5`). Junto al campo aparece la unidad del producto, p. ej. `m`.
2. Presiona **Enter** (en iPhone/iPad, **Ir**), **Tab**, o haz clic fuera del campo.

El total de la línea, el **Subtotal**, el **IVA (16%)** y el **Total** se actualizan
solos. La cantidad se captura en la unidad del producto que aparece junto al campo.

## Elegir el margen

Cada cotización se calcula con un [[margen]] que eliges de una lista, p. ej.
*Estándar (12.34%)*. Una cotización nueva empieza con el margen predeterminado.

1. En **Margen**, arriba de las líneas, elige otro de la lista.
2. Los precios y totales se recalculan solos.
3. [Guarda el borrador](#guardar-el-borrador) para conservar el cambio.

El margen aplica a todos los productos del catálogo en la cotización; las
[líneas libres](#agregar-una-línea-libre) no lo usan. El margen no aparece en el PDF.

### Usar un margen propio (cotizaciones CCS)

Esto solo está disponible en las cotizaciones de la serie **QS** (CCS y AC); las demás
series usan únicamente los márgenes de la lista.

Junto a **Margen** siempre ves el margen elegido de dos maneras (con un margen de la
lista, solo de lectura). Si ninguno te sirve, elige **Personalizado…** y los dos campos
se desbloquean:

- **Margen (%)**: cualquier porcentaje de 0 a 99.9999, p. ej. `31.5`.
- **Precio CCS 30% ($/kg)**: el precio de venta por kilo del cobre que ese margen
  implica (costo del material ÷ (1 − margen)). Solo aparece si el catálogo tiene el
  material *CCS 30%*.

Escribe en cualquiera de los dos y el otro se actualiza al instante: si cambias el
porcentaje, el precio por kilo se recalcula, y si escribes un precio por kilo, el
porcentaje se recalcula. El precio no puede ser menor que el costo del material.
Al terminar, los precios y totales se recalculan; [guarda el borrador](#guardar-el-borrador)
para conservar el margen. Al emitir, la cotización queda con el margen *Personalizado*
y su porcentaje. Si vuelves a elegir uno de la lista, el margen propio se descarta.

> La lista de márgenes la mantiene un administrador en Ajustes. Si cambia el porcentaje
> de tu margen, tu borrador toma el valor nuevo; si lo retira, tienes que
> [elegir otro](#problemas-comunes) antes de emitir.

## Agregar una línea libre

Para conceptos que no están en el catálogo: un flete, un producto especial, un servicio.

1. Haz clic en **Línea libre**. Aparece una línea nueva.
2. Escribe la **Descripción**; así aparecerá en el PDF.
3. Escribe el **Precio unitario** en pesos, sin signo `$` (p. ej. `123.45`).
4. Ajusta la **Cantidad** si no es 1.

## Quitar una línea

Haz clic en **Eliminar** en la fila de la línea. No pide confirmación.

> ¿Quitaste una línea por error? Recarga la página **sin guardar**: vuelves a lo último
> que guardaste.

## Guardar el borrador

Los cambios en las líneas **no se guardan solos**.

1. Haz clic en **Guardar borrador**.
2. Verás *"Borrador guardado."*

> Si sales de la página o la recargas sin guardar, pierdes los cambios desde la última
> vez que guardaste.

> Si alguna línea tiene un error (un mensaje en rojo debajo de ella), no se guarda nada
> hasta corregirla. Ver [problemas comunes](#problemas-comunes).

> **Los precios de un borrador no son definitivos.** Cada vez que abres un borrador, sus
> precios se recalculan con el catálogo, el margen elegido y los ajustes vigentes. Si
> un administrador cambia el costo de un producto, el precio de un material o el
> porcentaje de tu margen, tu borrador toma los precios nuevos. Debajo de los totales verás el precio
> de cada material que usan tus líneas y cuándo se actualizó (p. ej. *CCS 30% $160.00/kg
> (actualizado hace 3 días)*).
> Los precios se fijan hasta que [emites](#emitir-la-cotización).

## Ver una vista previa del PDF

1. Primero [guarda el borrador](#guardar-el-borrador): la vista previa usa lo último
   guardado, no lo que está en pantalla.
2. Haz clic en **Descargar PDF**. Se abre en otra pestaña.

> La vista previa **no es el documento final**: no tiene fecha de vigencia y sus precios
> pueden cambiar hasta que emitas. No se la envíes al cliente; emite primero.

## Emitir la cotización

Cuando la cotización está lista para enviarse al cliente:

1. Revisa las líneas y los totales.
2. Haz clic en **Emitir cotización**. Guarda las líneas que están en pantalla y emite en
   un solo paso: no hace falta guardar antes. **No hay paso de confirmación**: al hacer
   clic, se emite.
3. Verás *"Cotización emitida."* y **Estado: emitida**.

Qué pasa al emitir:

- Se congelan los precios, el margen y los términos y condiciones. La
  cotización ya no se puede editar; aunque después cambien los ajustes o el catálogo,
  lo que ves en la página y en el PDF sigue siendo lo que se emitió.
- El PDF queda definitivo. Cada vez que lo descargues tendrá los mismos precios,
  cliente, vendedor, fechas y términos con que se emitió, aunque después cambien los
  ajustes, el catálogo o los datos del cliente.
- La [[vigencia]] es de 30 días a partir de hoy; aparece en el PDF y en la página.

> Para emitir, la cotización necesita al menos una línea, ninguna línea con error y un
> margen disponible.

## Descargar y enviar el PDF

1. Abre la cotización: en **Cotizaciones**, haz clic en su folio.
2. Haz clic en **Descargar PDF**. Se abre en otra pestaña; desde ahí guárdalo o
   imprímelo.
3. Envíalo al cliente por tu medio habitual (correo, WhatsApp). La app no envía
   correos.

## Revisar una cotización emitida

Cuando el cliente pide un cambio (otra cantidad, otro producto). Una cotización emitida
no se modifica: se crea una revisión.

1. Abre la cotización emitida.
2. Haz clic en **Revisar**.
3. La app crea un borrador nuevo con el mismo folio más `-R1` (p. ej. `QA0012-R1`),
   con las mismas líneas, y te lleva a él. Arriba dice *"Esta es una revisión de
   QA0012."*
4. Edítalo como cualquier borrador: [cambia cantidades](#cambiar-la-cantidad-de-una-línea),
   [agrega](#agregar-productos-por-sku) o [quita](#quitar-una-línea) líneas.
5. [Emítelo](#emitir-la-cotización) cuando esté listo y [envía el PDF nuevo](#descargar-y-enviar-el-pdf).

Qué pasa al revisar:

- La original pasa a [[revisada]]. Queda intacta, con su PDF original, y muestra un
  enlace a la revisión.
- Una revisión de la `-R1` (ya emitida) será `-R2`, y así sucesivamente. Solo se puede
  revisar la versión emitida más reciente.
- No se puede deshacer: al hacer clic en **Revisar**, la original queda como revisada.

> **Importante:** la revisión es un borrador. Empieza con el mismo margen que la
> original, pero sus precios se recalculan con el catálogo, el porcentaje de ese margen
> y los ajustes **actuales**, no con los de la original. Revisa los precios antes de
> emitirla.

## Ver el detalle de una cotización

Desde **Cotizaciones**, haz clic en el folio de cualquier cotización que no sea
[[borrador]]. Debajo de las líneas y los totales verás:

- **Estado** de la cotización.
- **Emitió**: quién la emitió.
- **Emitida** y **Vigencia**: las fechas.
- **Margen**: el margen con el que se cotizó, congelado al emitir.
- **Utilidad**: cuánto gana la cotización antes de IVA (el margen aplicado a las líneas
  de catálogo; las líneas libres no cuentan porque no tienen costo).

## Comentar una cotización

Toda cotización, en cualquier estado, tiene una caja de **Comentarios** al final de la
página, debajo de **Descargar PDF** y **Volver a cotizaciones**. Sirve para dar
seguimiento (*"el cliente pidió otro precio"*, *"llamar el lunes"*).

1. Abre la cotización.
2. Escribe en **Agregar un comentario** y haz clic en **Comentar**.
3. Tu comentario aparece en la tabla de abajo, con tu nombre y la fecha. Los más nuevos
   van arriba.

Cualquier usuario puede comentar. Los comentarios no se editan ni se borran.

## Buscar una cotización

1. En el menú, entra a **Cotizaciones**.
2. Escribe el folio o el nombre del cliente en *"Buscar por folio o cliente…"*.
3. Para ordenar, haz clic en el encabezado **Folio**, **Cliente**, **Autor**,
   **Estado**, **Total** o **Fecha**. Otro clic invierte el orden. Sin ordenar, van de la más
   reciente a la más antigua y las cotizaciones de alumbrado (QI) quedan al final.
4. Para acotar más, [filtra por columna](README.md#filtrar-las-tablas) con el embudo de cada encabezado.
5. Haz clic en el folio para abrir la cotización.

## Problemas comunes

| Mensaje | Qué significa | Qué hacer |
|---|---|---|
| *Selecciona un cliente.* | No elegiste cliente al crear la cotización. | Elige uno. Si no aparece, [dalo de alta](clientes.md#dar-de-alta-un-cliente). |
| *Selecciona una serie de folio.* | No elegiste QA, QS o QI. | Elige una ([¿cuál?](#crear-una-cotización)). |
| *Cantidad inválida.* | La cantidad está vacía, es cero o negativa, o no es un número. | Escribe un número mayor que 0. |
| *Descripción obligatoria.* | Una línea libre no tiene descripción. | Escríbela, o quita la línea. |
| *Precio inválido.* | Una línea libre tiene el precio vacío o con letras o signos. | Escribe solo el número, p. ej. `123.45`. |
| *Elige un margen para esta cotización.* | La cotización no tiene margen (p. ej. una revisión de una cotización antigua). | [Elige uno](#elegir-el-margen) en **Margen**. |
| *El margen elegido ya no está disponible; elige otro.* | Un administrador retiró el margen de la cotización. | [Elige otro](#elegir-el-margen) en **Margen**. |
| *Elige un margen disponible para esta cotización.* | Aparece en las líneas del catálogo mientras la cotización no tiene un margen disponible. | Igual: elige un margen en **Margen**. |
| Al hacer clic en **Emitir cotización** la página se recarga con un mensaje sobre el margen. | El margen de la cotización no está disponible. | Elige un margen en **Margen** y vuelve a emitir. |
| *Este producto no tiene datos de precio configurados.* | El producto no tiene costo ni materiales. | [Corrige el producto](productos.md#editar-un-producto) o usa una [línea libre](#agregar-una-línea-libre). |
| *Producto no encontrado.* | El producto se eliminó del catálogo después de agregarlo al borrador. | Quita la línea y agrega el producto correcto. |
| *esta cotización ya no es editable* | La cotización ya fue emitida (p. ej. la emitiste desde otra pestaña). | Ábrela de nuevo. Para cambiarla, usa [Revisar](#revisar-una-cotización-emitida). |
| Al hacer clic en **Emitir cotización** la página se recarga sin mensaje. | La cotización no tiene líneas. | Agrega al menos una línea. |
