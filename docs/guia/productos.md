# Productos

El catálogo del que salen los precios de las cotizaciones. Cualquier usuario puede dar de
alta un producto, pero una vez creado **solo un administrador** puede editarlo (precio,
costo, materiales, conversiones o cualquier otro dato) o eliminarlo, porque cualquier
cambio aquí afecta los precios de **todos** los borradores que incluyen el producto.

Si eres vendedor, en la lista verás **Ver** en lugar de **Editar** y **Eliminar**: el
producto se abre en solo lectura, con *"Solo un administrador puede modificar o eliminar
este producto."* Si encuentras un dato equivocado, pídele a un administrador que lo
corrija.

## Buscar un producto

1. En el menú **Catálogos**, elige **Productos**.
2. Escribe en *"Buscar por SKU o descripción…"*. La lista se filtra mientras escribes.
3. Para ordenar, haz clic en el encabezado de una columna (**SKU**, **Descripción**,
   **Familia**, **Costo**, **Unidad**, **Datos SAT**).
4. Para acotar más, [filtra por columna](README.md#filtrar-las-tablas) con el embudo de cada encabezado.

La columna **Datos SAT** muestra las dos [claves para facturar](#datos-para-facturar-sat)
del producto (p. ej. *26121600 · MTR*) o, si le falta alguna, *"Sin datos para
facturar"*. Para ver solo esos productos, haz clic en el botón **Sin datos para
facturar** junto a **Nuevo producto** (o filtra la columna **Datos SAT**).

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
administrador en [Configuración del sistema](administracion.md#materiales). El margen lo elige el vendedor
en cada cotización, de la [lista de márgenes](administracion.md#márgenes).

Todo es en pesos mexicanos: si el proveedor cobra en dólares, captura el costo ya
convertido a pesos.

*Ejemplos:*

- Un producto con costo `45.00`, en una cotización con margen de 30 %, se cotiza a
  45.00 ÷ 0.70 = **$64.29**.
- El Cable CCS 30% ALAMBRE 4 lleva 0.1723 kg de *CCS 30%* por metro. Con el material a
  $160.00/kg y margen de 29.55 %: 0.1723 × 160 = 27.57 de costo; 27.57 ÷ 0.7045 =
  **$39.13** por metro.

- En la familia de la serie **QG** (Alumoclad), el costo es por kilo y el precio es por
  metro: ALC-C7#8 a $95.00/kg con **Peso** de 0.3896 kg/m y margen de 10 %: 95 × 0.3896 =
  37.01 de costo por metro; 37.01 ÷ 0.90 = **$41.12** por metro.

> Fuera de la serie QG, el **Peso (kg/m)** del producto es solo informativo: no cambia el
> precio. Lo que cuenta para el precio es la cantidad de cada material. En QG el peso es
> obligatorio, porque de él sale el precio por metro.

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
   **Peso (kg/m)**, solo informativo, p. ej. `0.123`. En la familia de la serie **QG**
   el costo va por kilo y el peso es obligatorio, p. ej. `0.3896`.
6. Opcional: elige la **Unidad** (p. ej. metro). Aparece junto a la cantidad en las
   cotizaciones.
7. Opcional, pero necesario para facturarlo: en **Datos para facturar (SAT)**, la clave
   de producto y la clave de unidad del SAT ([ver abajo](#datos-para-facturar-sat)).
8. Haz clic en **Guardar**. Antes de crearlo aparece un cuadro: *"¿Revisaste que todos
   los datos sean correctos? Una vez creado, solo un administrador podrá modificarlo."*
   - **Sí, crear producto**: lo crea. Verás *"Producto guardado."*
   - **Regresar a revisar**: cierra el cuadro sin guardar y deja lo que escribiste.

## Editar un producto

Solo un administrador. Por ejemplo, para actualizar un precio o un costo.

1. [Busca el producto](#buscar-un-producto).
2. Haz clic en **Editar** en su fila.
3. Cambia lo necesario y haz clic en **Guardar**.

El cambio aplica a las cotizaciones nuevas y a los borradores que ya incluyen el
producto (toman el precio nuevo la próxima vez que se abren). Las cotizaciones emitidas
no cambian.

## Datos para facturar (SAT)

Cada producto lleva dos claves del SAT, las que salen en la factura. Son opcionales para
dar de alta y cotizar el producto, pero sin las dos **no se podrá facturar**. Las captura
quien da de alta el producto; después, como el resto del producto, solo un administrador
las cambia. Confírmalas con el contador.

- **Clave de producto o servicio SAT**: 8 dígitos del catálogo del SAT, p. ej.
  `26121600`. Escríbela si la sabes, o búscala en **Buscar la clave por descripción o por
  número**: escribe una palabra (p. ej. `cable`) o el principio de la clave, y haz clic en
  la que corresponda. Debajo del campo ves la descripción oficial de la clave (p. ej.
  *Cables eléctricos y accesorios*) para confirmar que es la correcta.
- **Clave de unidad SAT**: cómo se factura el producto, p. ej. *MTR · Metro*. La lista
  trae primero las más usadas (*H87 · Pieza*, *KGM · Kilogramo*, *MTR · Metro*, *XBX ·
  Caja*, *XRO · Rollo*, *XRL · Carrete*, *XPK · Paquete*, *KT · Kit*, *PR · Par*, *LTR ·
  Litro*, *E48 · Unidad de servicio*, *ACT · Actividad*). Para otra, búscala en **Buscar
  otra unidad en el catálogo del SAT** y haz clic en ella.

Las dos se eligen producto por producto: no dependen de la familia ni de la **Unidad**
del producto (esa se queda para el uso interno de la app). Dos productos de la misma
familia pueden llevar claves distintas, p. ej. uno por metro y otro por pieza.

Solo se aceptan claves que existan en los catálogos del SAT (CFDI 4.0). Si un producto que
se cotiza por metro con su peso (como los de las series QS y QG) se factura en kilos
(**KGM**), necesita su peso por metro para convertir los metros a kilos: o su **Peso
(kg/m)**, o sus [materiales](#materiales-de-un-producto) en kg (p. ej. un cable CCS, que
lleva 0.1723 kg de CCS 30% por metro). Un producto nuevo todavía no tiene materiales: si
su peso sale de ellos, guárdalo primero con otra clave de unidad, agrégale sus materiales
y después elige **KGM**.

Al crear una factura, cada partida toma las claves de su producto
([Facturas](facturas.md#crear-una-factura)).

## Materiales de un producto

Para los productos cuyo costo sale de lo que están hechos (p. ej. Cable CCS & AC). Solo un
administrador los agrega o quita; un vendedor los ve en la página del producto. En la
página de edición del producto, sección **Materiales**:

1. En **Material**, elige el material (p. ej. *CCS 30% (kg)*). Los materiales los da de
   alta un administrador en [Configuración del sistema](administracion.md#materiales).
2. En **Cantidad por unidad del producto**, escribe cuánto material lleva **una** unidad
   del producto, en la unidad del material. Por ejemplo, `0.1723` = 0.1723 kg por metro.
3. Haz clic en **Agregar material**.

Para quitar un material, haz clic en **Eliminar** en su fila. Para cambiar la cantidad,
elimínalo y vuelve a agregarlo.

El cambio aplica a los borradores que incluyen el producto; las cotizaciones emitidas no
cambian.

## Conversiones de unidades

Solo un administrador. En la página de edición de un producto, sección **Conversiones de
unidades**:

1. En **De**, elige la unidad de origen; en **A**, la de destino.
2. En **Tasa**, escribe cuántas unidades "A" hay en 1 unidad "De". Por ejemplo, De
   `caja` A `m` con Tasa `100` significa que una caja tiene 100 m.
3. Haz clic en **Agregar conversión**.

Para quitar una conversión, haz clic en **Eliminar** en su fila.

> Por ahora las cotizaciones capturan la cantidad en la unidad del producto; las
> conversiones quedan registradas para usarlas más adelante. Las unidades disponibles
> las da de alta un administrador en [Unidades](administracion.md#unidades).

## Eliminar un producto

Solo un administrador.

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
| *El peso es obligatorio en la familia Alumoclad: el precio por metro sale del costo por kg × el peso. Escríbelo en kg/m, p. ej. 0.3896.* | Escribe el peso en kg por metro, mayor que 0. |
| *La clave de producto 12345678 no existe en el catálogo del SAT. Búscala por su descripción.* | Busca la clave en **Buscar la clave por descripción o por número** y elígela de la lista. |
| *La clave de unidad XYZ no existe en el catálogo del SAT.* | Elige la unidad de la lista o búscala en el catálogo. |
| *Para facturar en kilos (KGM) un producto que se cotiza por metro hace falta su peso, que convierte los metros a kilos: escríbelo en kg/m, p. ej. 0.3896, o agrégale sus materiales en kg.* | Escribe el **Peso (kg/m)**; o, si el peso sale de sus materiales, guárdalo con otra clave de unidad (p. ej. *MTR · Metro*), agrega sus materiales en kg y vuelve a elegir **KGM**. |
| *Cantidad inválida; usa un número positivo, p. ej. 0.1723.* | En Materiales, escribe la cantidad como número mayor que 0. |
| *Este producto ya tiene ese material; elimínalo y vuelve a agregarlo para cambiar la cantidad.* | Elimina la fila del material y agrégalo con la cantidad nueva. |
