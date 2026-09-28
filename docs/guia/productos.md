# Productos

El catálogo del que salen los precios de las cotizaciones. Vendedores y administradores
pueden editarlo, así que cualquier cambio aquí afecta los precios de **todos** los
borradores que incluyen el producto.

## Buscar un producto

1. En el menú, entra a **Productos**.
2. Escribe en *"Buscar por SKU o descripción…"*. La lista se filtra mientras escribes.
3. Para ordenar, haz clic en el encabezado de una columna (**SKU**, **Descripción**,
   **Familia**, **Costo**, **Unidad**).
4. Para acotar más, [filtra por columna](README.md#filtrar-las-tablas) con el embudo de cada encabezado.

En la columna **Costo** verás el costo del producto, sin margen: su costo fijo, seguido
de *"+ materiales"* si además tiene materiales, o solo *"materiales"* si su costo sale
de sus [materiales](#materiales-de-un-producto) (ver abajo).

## Cómo se calcula el precio de un producto

Todos los productos se cotizan igual:

> **Precio = (Costo + costo de sus materiales) ÷ (1 − [[margen]] de la cotización)**

| Tiene | Típico de |
|---|---|
| **Costo** | Cable CCA, Alumbrado |
| [Materiales](#materiales-de-un-producto) | Cable CCS & AC |

El costo de los materiales es, por cada [[material]], la cantidad que lleva una unidad
del producto × el precio del material. Los precios de los materiales los captura un
administrador en [Ajustes](administracion.md#materiales). El margen lo elige el vendedor
en cada cotización, de la [lista de márgenes](administracion.md#márgenes).

Todo es en pesos mexicanos: si el proveedor cobra en dólares, captura el costo ya
convertido a pesos.

*Ejemplos:*

- Un producto con costo `45.00`, en una cotización con margen de 30 %, se cotiza a
  45.00 ÷ 0.70 = **$64.29**.
- El Cable CCS 30% ALAMBRE 4 lleva 0.1723 kg de *CCS 30%* por metro. Con el material a
  $160.00/kg y margen de 29.55 %: 0.1723 × 160 = 27.57 de costo; 27.57 ÷ 0.7045 =
  **$39.13** por metro.

> El **Peso (kg/m)** del producto es solo informativo: no cambia el precio. Lo que
> cuenta para el precio es la cantidad de cada material.

## Dar de alta un producto

1. Entra a **Productos** y haz clic en **Nuevo producto**.
2. Escribe el **SKU** (obligatorio; ver [[SKU]]).
3. Escribe la **Descripción** (obligatoria). Es el texto que aparece en la cotización y
   en el PDF.
4. Elige la **Familia** (obligatoria; ver [[familia]]).
5. Escribe el **Costo** en pesos, sin margen, solo con números y sin `$`, p. ej. `45.00`
   ([¿cómo se usa?](#cómo-se-calcula-el-precio-de-un-producto)). Si el producto se
   cotiza por sus materiales, déjalo vacío y agrégale los
   [materiales](#materiales-de-un-producto) después de guardarlo. Opcional: el
   **Peso (kg/m)**, solo informativo, p. ej. `0.123`.
6. Opcional: elige la **Unidad** (p. ej. metro). Aparece junto a la cantidad en las
   cotizaciones.
7. Haz clic en **Guardar**. Verás *"Producto guardado."*

## Editar un producto

Por ejemplo, para actualizar un precio o un costo.

1. [Busca el producto](#buscar-un-producto).
2. Haz clic en **Editar** en su fila.
3. Cambia lo necesario y haz clic en **Guardar**.

El cambio aplica a las cotizaciones nuevas y a los borradores que ya incluyen el
producto (toman el precio nuevo la próxima vez que se abren). Las cotizaciones emitidas
no cambian.

## Materiales de un producto

Para los productos cuyo costo sale de lo que están hechos (p. ej. Cable CCS & AC). En la
página de edición del producto, sección **Materiales**:

1. En **Material**, elige el material (p. ej. *CCS 30% (kg)*). Los materiales los da de
   alta un administrador en [Ajustes](administracion.md#materiales).
2. En **Cantidad por unidad del producto**, escribe cuánto material lleva **una** unidad
   del producto, en la unidad del material. Por ejemplo, `0.1723` = 0.1723 kg por metro.
3. Haz clic en **Agregar material**.

Para quitar un material, haz clic en **Eliminar** en su fila. Para cambiar la cantidad,
elimínalo y vuelve a agregarlo.

El cambio aplica a los borradores que incluyen el producto; las cotizaciones emitidas no
cambian.

## Conversiones de unidades

En la página de edición de un producto, sección **Conversiones de unidades**:

1. En **De**, elige la unidad de origen; en **A**, la de destino.
2. En **Tasa**, escribe cuántas unidades "A" hay en 1 unidad "De". Por ejemplo, De
   `caja` A `m` con Tasa `100` significa que una caja tiene 100 m.
3. Haz clic en **Agregar conversión**.

Para quitar una conversión, haz clic en **Eliminar** en su fila.

> Por ahora las cotizaciones capturan la cantidad en la unidad del producto; las
> conversiones quedan registradas para usarlas más adelante. Las unidades disponibles
> las da de alta un administrador en [Unidades](administracion.md#unidades).

## Eliminar un producto

1. [Busca el producto](#buscar-un-producto).
2. Haz clic en **Eliminar** en su fila y confirma *"¿Eliminar este producto?"*.

Qué pasa al eliminar:

- Desaparece del catálogo y de la búsqueda de productos en las cotizaciones.
- Las cotizaciones emitidas no cambian.
- Los borradores que ya lo tenían muestran *"Producto no encontrado."* en esa línea;
  hay que quitarla antes de guardar o emitir.

## Problemas comunes

| Mensaje | Qué hacer |
|---|---|
| *El SKU es obligatorio.* / *La descripción es obligatoria.* | Llena el campo. |
| *Selecciona una familia.* | Elige una familia de la lista. |
| *Costo inválido; usa un número, p. ej. 123.45.* (y el equivalente de Peso) | Escribe solo el número, sin `$` ni comas. |
| *Cantidad inválida; usa un número positivo, p. ej. 0.1723.* | En Materiales, escribe la cantidad como número mayor que 0. |
| *Este producto ya tiene ese material; elimínalo y vuelve a agregarlo para cambiar la cantidad.* | Elimina la fila del material y agrégalo con la cantidad nueva. |
