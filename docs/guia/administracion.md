# Administración

Solo para cuentas con rol **Administrador**. Estas opciones están en el menú **Catálogos**,
debajo de Productos y Clientes: **Usuarios**, **Familias**, **Tipos de cotización**,
**Unidades** y **Configuración del sistema**.

En **Configuración del sistema** están los dos datos de los que salen los precios, además del costo de
cada producto: los [márgenes](#márgenes) y los [materiales](#materiales). Ver
[cómo se calcula el precio de un producto](productos.md#cómo-se-calcula-el-precio-de-un-producto).

Efecto de cualquier cambio en Configuración del sistema:

- **Inmediato** en las cotizaciones nuevas y en **todos los borradores** (toman los
  valores nuevos la próxima vez que se abren o recalculan).
- **Ninguno** en las cotizaciones emitidas: conservan los precios con que se emitieron.

> Avisa a los vendedores cuando cambies un margen o el precio de un material: los precios
> de sus borradores van a cambiar.

Todos los precios son en pesos mexicanos. Si un proveedor te cobra en dólares, captura
el costo ya convertido a pesos.

## Márgenes

Los [[margen|márgenes]] son una lista de opciones con nombre (p. ej. *Estándar (12.34%)*).
Cada cotización se calcula con **una** de ellas, que el vendedor elige al armarla. Solo
un administrador puede cambiar la lista. Están en **Configuración del sistema**, en **Márgenes**.

El margen es sobre el precio de venta: el precio es costo ÷ (1 − margen). Con 35 %, un
costo de $65.00 se cotiza a $100.00. Afecta a todos los productos del catálogo; las
líneas libres no lo usan.

**Agregar un margen**

1. En **Nuevo margen**, escribe el **Nombre** (p. ej. `Distribuidor`) y el
   **Margen (%)** como porcentaje, sin el signo: `12.34` = 12.34 %. Acepta hasta 4
   decimales y debe ser menor que 100.
2. Haz clic en **Agregar**.

**Cambiar el nombre o el porcentaje**

1. En la fila del margen, cambia el nombre o el porcentaje.
2. Haz clic en **Guardar** de esa fila.

> **Los borradores siguen al margen.** Si cambias el porcentaje, todos los borradores
> que usan ese margen se recalculan con el nuevo valor la próxima vez que se abren. Las
> cotizaciones emitidas no cambian: guardan el nombre y el porcentaje con que se
> emitieron.

**Margen predeterminado**

Las cotizaciones nuevas empiezan con el margen **Predeterminado**. Para cambiarlo, haz
clic en **Hacer predeterminado** en otro margen.

**Retirar o restaurar un margen**

- **Retirar** quita el margen de la lista que ven los vendedores. No se borra: las
  cotizaciones emitidas con él lo siguen mostrando. Un borrador que lo tenía elegido
  muestra *"El margen elegido ya no está disponible; elige otro."* y no se puede emitir
  hasta elegir otro.
- No se puede retirar el predeterminado; primero haz predeterminado otro.
- **Restaurar** lo vuelve a poner en la lista.

## Materiales

Los [[material|materiales]] (p. ej. *CCS 30%*) y su precio. Los productos hechos de un
material se cotizan con la cantidad que llevan × el precio del material, más el margen
de la cotización. Ver [materiales de un producto](productos.md#materiales-de-un-producto).
Están en **Configuración del sistema**, en **Materiales**.

El precio es el **costo** del material, sin margen: el margen lo pone cada cotización.

**Actualizar el precio de un material**

1. En la fila del material, cambia el **Precio** (pesos por unidad, p. ej. `160.00`
   por kg).
2. Haz clic en **Guardar** de esa fila.

La columna **Actualizado** dice cuándo cambió por última vez (p. ej. *actualizado hace
3 días*). Los vendedores ven lo mismo debajo de los totales de sus cotizaciones.

> Todos los borradores con productos hechos de ese material se recalculan con el precio
> nuevo la próxima vez que se abren. Las cotizaciones emitidas no cambian.

**Agregar un material**

1. En **Nuevo material**, escribe el **Nombre**, elige la **Unidad** en que se mide
   (p. ej. Kilogramo) y escribe el **Precio** por esa unidad.
2. Haz clic en **Agregar**.

La unidad no se puede cambiar después, porque las cantidades de los productos están
escritas en ella. Los materiales no se pueden eliminar.

## Facturación

Al final de **Configuración del sistema**, **Facturación** muestra si la app puede timbrar
con Facturoporti. Solo se consulta; los datos (token y certificado de sello digital) se
instalan en el servidor ([cómo](../NAS_OPERATIONS.md#secrets)).

- **Ambiente**: *Producción* o *Pruebas* (los timbres de pruebas no son válidos).
- **Estado**: *Lista para timbrar*, o *Apagada* con la razón (por ejemplo, *no está
  configurada* o *el certificado venció el …*).
- **Certificado**: el RFC, el número y la fecha de vencimiento del certificado.

## Ambiente de pruebas

Hay una copia de la app para probar sin tocar los datos reales, en
`http://192.168.3.169:8091` (solo desde la red de la oficina). Arriba de cada página dice
*"Ambiente de pruebas"* y sus PDF llevan la marca *PRUEBA*. Sus datos son una copia de los
reales del día en que se copiaron; lo que hagas ahí no cuenta y se borra en la siguiente
copia. Entras con tu mismo usuario y contraseña.

## Cambiar el rol de un usuario

1. En el menú **Catálogos**, elige **Usuarios**.
2. En la fila del usuario, elige **Administrador** o **Vendedor**.
3. Haz clic en **Guardar rol**.

No puedes cambiar tu propio rol.

## Deshabilitar o habilitar un usuario

1. En **Usuarios**, busca la fila del usuario.
2. Haz clic en **Deshabilitar** y confirma *"¿Deshabilitar a este usuario?"*. El usuario
   ya no puede iniciar sesión; sus cotizaciones se conservan.
3. Para devolverle el acceso, haz clic en **Habilitar**.

No puedes deshabilitarte a ti mismo.

## Dar de alta un usuario o restablecer una contraseña

Esto **no** se hace desde la app, sino por línea de comandos en el servidor. Quien
administra el servidor sigue el procedimiento de
[NAS_OPERATIONS.md](../NAS_OPERATIONS.md). Por ejemplo, para dar de alta un vendedor:

```bash
ssh cladex-nas 'cd /volume1/docker/cladex && docker compose run --rm cladex user add <usuario> --name "Nombre Completo" --role vendedor'
```

Para restablecer una contraseña se usa `user passwd <usuario>` de la misma forma; esto
también cierra las sesiones abiertas de ese usuario.

Para cambiar el nombre con que aparece un usuario (y con que sale como vendedor en las
cotizaciones que emita de ahí en adelante):

```bash
ssh cladex-nas 'cd /volume1/docker/cladex && docker compose run --rm cladex user rename <usuario> --name "Nombre Apellido"'
```

Las cotizaciones ya emitidas conservan el nombre con que se emitieron.

Al dar de alta o restablecer, el comando genera una contraseña y la muestra **una sola vez** en
pantalla. Entrégasela al usuario por un medio privado y pídele que la
[cambie](acceso.md#cambiar-tu-contraseña) desde **Mi cuenta** en cuanto entre.

## Unidades

Las unidades (metro, caja, pieza…) que se pueden asignar a los productos.

1. En el menú **Catálogos**, elige **Unidades**. Arriba ves la lista actual.
2. En **Nueva unidad**, escribe el **Código** (corto, p. ej. `caja`) y el **Nombre**
   (p. ej. `Caja`).
3. Haz clic en **Agregar**.

Las unidades no se pueden editar ni eliminar desde la app.

## Familias

Una [[familia]] es una línea de producto del catálogo (CCA, CCS & AC, ABASTILUM…): cada
producto pertenece a una. La serie de folio, los términos y cómo se ve el PDF no son de
la familia sino de cada [tipo de cotización](#tipos-de-cotización). **QL** es una
familia especial sin productos: la usa el tipo QL, que solo cotiza
[líneas libres](cotizaciones.md#agregar-una-línea-libre).

### Crear una familia

1. En el menú **Catálogos**, elige **Familias**. Arriba ves la lista actual, con los
   tipos de cotización de cada una.
2. En **Nueva familia**, escribe el **Nombre** y haz clic en **Crear familia**.
3. Para cotizar sus productos con una serie propia, crea después un
   [tipo de cotización](#crear-un-tipo-de-cotización) de esa familia.

### Clave SAT de una familia

En **Familias**, haz clic en **Editar** en la fila de la familia, escribe la **Clave de
producto SAT** (8 dígitos, p. ej. `26121600`) y haz clic en **Guardar**. Las partidas de
las facturas de esa familia empiezan con esa clave; en cada factura se puede cambiar.
Confírmala con el contador.

El nombre de una familia no se puede cambiar, y las familias no se eliminan desde la app.

Mensajes de error:

| Mensaje | Qué hacer |
|---|---|
| *El nombre es obligatorio.* | Escribe el nombre de la familia. |
| *Ya existe una familia con este nombre.* | Usa otro nombre. |
| *La clave SAT son 8 dígitos, p. ej. 26121600.* | Escribe solo los 8 dígitos. |

## Tipos de cotización

Un [[tipo de cotización]] es una clase de cotización: su [[serie]] de folio (QA, QS,
QI…), su numeración, sus *Términos y condiciones* y cómo se ve su PDF. Al
[crear una cotización](cotizaciones.md#crear-una-cotización) se elige su tipo. Están
en el menú **Catálogos**, **Tipos de cotización**; la lista muestra el siguiente folio
de cada uno.

### Crear un tipo de cotización

1. En **Tipos de cotización**, ve a **Nuevo tipo de cotización**.
2. En **Serie**, escribe una Q seguida de 1 a 5 letras (p. ej. `QCCA`). Es el prefijo de
   sus folios (`QCCA0001`).
3. Opcional: el **Nombre** aparece junto a la serie al elegir el tipo (p. ej.
   *QCCA — Cable CCA*).
4. En **Familia de sus productos**, elige la familia que cotiza. El buscador de productos
   de sus cotizaciones la muestra, y sus facturas toman su clave SAT.
5. En **Términos y condiciones**, escribe un término por línea; se imprimen en el PDF.
   Si quieres que cada cotización pida un tiempo de entrega, escribe
   `{tiempo_de_entrega}` donde debe ir, p. ej. `Tiempo de entrega: {tiempo_de_entrega}`.
   El vendedor tendrá que escribirlo antes de [emitir](cotizaciones.md#tiempo-de-entrega).
   Igual con `{moneda}`: el vendedor elige MXN o USD y el texto pasa a *pesos mexicanos
   (MXN)* o *dólares americanos (USD)*, p. ej. `Precios en {moneda}, no incluyen IVA`.
6. Elige el **Membrete del PDF** y marca lo que aplique:
   - *Solo líneas libres*: sin productos de catálogo, como QL. No se puede cambiar
     después.
   - *Precio por kg*: el costo de sus productos es por kilo y se cotizan por metro, como
     QG. Sus productos piden el peso (kg/m).
   - *"Otro margen" como precio por kg del cobre*, como QS.
   - *PDF sin la dirección de Cladex*.
   - *Al final de la lista de cotizaciones*.
7. Haz clic en **Crear tipo**. Aparece de inmediato en **Nueva cotización**.

### Cambiar un tipo de cotización

1. En **Tipos de cotización**, haz clic en **Editar** en su fila. Arriba dice cuál será
   su siguiente folio.
2. Cambia lo que necesites y haz clic en **Guardar**.

Puedes cambiar su **serie**, p. ej. de `QA` a `QCCA`:

- Las cotizaciones que ya tienen folio lo **conservan** (`QA0105` sigue siendo `QA0105`).
- La numeración **continúa** con la serie nueva: si la siguiente iba a ser `QA0106`,
  será `QCCA0106`.
- Una serie que ya tiene o usó **otro** tipo no se puede usar, para que nunca haya dos
  cotizaciones con el mismo folio.

Los cambios de términos, membrete y lo demás se ven en los borradores y en las
cotizaciones que se emitan de ahora en adelante. Las ya emitidas conservan los términos y
el PDF con que se emitieron.

> Los términos de **QL** incluyen la moneda (`{moneda}`) y el tiempo de entrega
> (`{tiempo_de_entrega}`), así que sus cotizaciones los piden. Si los editas, conserva
> esas líneas para que se sigan pidiendo. Los tipos de solo líneas libres (como QL) además
> piden una [vigencia](cotizaciones.md#moneda-y-vigencia-en-ql) escrita a mano.

Los tipos de cotización no se eliminan desde la app.

Mensajes de error:

| Mensaje | Qué hacer |
|---|---|
| *La serie es una Q seguida de 1 a 5 letras, p. ej. QCCA.* | Escribe algo como `QCCA`, sin números ni espacios. |
| *Esa serie ya la tiene o la usó otro tipo de cotización; elige otra.* | Elige otra serie. |

