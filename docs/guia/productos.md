# Productos

El catálogo del que salen los precios de las cotizaciones. Vendedores y administradores
pueden editarlo, así que cualquier cambio aquí afecta los precios de **todos** los
borradores que incluyen el producto.

## Buscar un producto

1. En el menú, entra a **Productos**.
2. Escribe en *"Buscar por SKU o descripción…"*. La lista se filtra mientras escribes.
3. Para ordenar, haz clic en el encabezado de una columna (**SKU**, **Descripción**,
   **Familia**, **Precio**, **Moneda**, **Unidad**).

En la columna **Precio** verás el precio fijo del producto, su costo seguido de
*"(costo)"*, o *"—"* si se cotiza por peso (ver abajo).

## Cómo se calcula el precio de un producto

Cada producto se cotiza de una de tres formas, según cuál de estos campos tenga lleno:

| Campo lleno | Precio en la cotización | Típico de |
|---|---|---|
| **Precio unitario** | Ese precio, tal cual | Alumbrado |
| **Costo** | Costo ÷ (1 − [[margen]] de la cotización) | Cable CCA |
| **Peso (kg/m)** | Peso × [[precio del cobre]] | Cable CCS & AC |

Si la **Moneda** del producto es USD, el resultado se multiplica por el [[tipo de cambio]].
El precio del cobre y el tipo de cambio los captura un administrador en
[Ajustes](administracion.md#actualizar-tipo-de-cambio-y-precio-del-cobre). El margen lo
elige el vendedor en cada cotización, de la [lista de márgenes](administracion.md#márgenes).

*Ejemplo:* un producto con costo `45.00` en una cotización con margen de 30 % se cotiza a
45.00 ÷ 0.70 = **$64.29**.

> **Llena solo uno de los tres campos.** Si hay más de uno, el sistema usa el primero en
> este orden (Precio unitario, luego Costo, luego Peso) e ignora los demás.

## Dar de alta un producto

1. Entra a **Productos** y haz clic en **Nuevo producto**.
2. Escribe el **SKU** (obligatorio; ver [[SKU]]).
3. Escribe la **Descripción** (obligatoria). Es el texto que aparece en la cotización y
   en el PDF.
4. Elige la **Familia** (obligatoria; ver [[familia]]).
5. Elige la **Moneda**: MXN o USD.
6. Llena **uno** de estos campos ([¿cuál?](#cómo-se-calcula-el-precio-de-un-producto)),
   solo con números, sin `$`:
   - **Precio unitario**, p. ej. `6.319872`
   - **Costo**, p. ej. `45.00`
   - **Peso (kg/m)**, p. ej. `0.123`
7. Opcional: elige la **Unidad** (p. ej. metro). Aparece junto a la cantidad en las
   cotizaciones.
8. Haz clic en **Guardar**. Verás *"Producto guardado."*

## Editar un producto

Por ejemplo, para actualizar un precio o un costo.

1. [Busca el producto](#buscar-un-producto).
2. Haz clic en **Editar** en su fila.
3. Cambia lo necesario y haz clic en **Guardar**.

El cambio aplica a las cotizaciones nuevas y a los borradores que ya incluyen el
producto (toman el precio nuevo la próxima vez que se abren). Las cotizaciones emitidas
no cambian.

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
| *Precio inválido; usa un número, p. ej. 123.45.* (y los equivalentes de Costo y Peso) | Escribe solo el número, sin `$` ni comas. |
