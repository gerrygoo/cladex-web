# Administración

Solo para cuentas con rol **Administrador**. Estas opciones aparecen en el menú como
**Usuarios**, **Unidades**, **Familias** y **Ajustes**.

En **Ajustes** están los dos datos de los que salen los precios, además del costo de
cada producto: los [márgenes](#márgenes) y los [materiales](#materiales). Ver
[cómo se calcula el precio de un producto](productos.md#cómo-se-calcula-el-precio-de-un-producto).

Efecto de cualquier cambio en Ajustes:

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
un administrador puede cambiar la lista. Están abajo en **Ajustes**, en **Márgenes**.

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
Están abajo en **Ajustes**, en **Materiales**.

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

## Cambiar el rol de un usuario

1. En el menú, entra a **Usuarios**.
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
ssh cladex-nas docker compose exec cladex /cladex user add <usuario> --name "Nombre Completo" --role vendedor
```

Para restablecer una contraseña se usa `user passwd <usuario>` de la misma forma; esto
también cierra las sesiones abiertas de ese usuario.

En ambos casos el comando genera una contraseña y la muestra **una sola vez** en
pantalla. Entrégasela al usuario por un medio privado y pídele que la
[cambie](acceso.md#cambiar-tu-contraseña) desde **Mi cuenta** en cuanto entre.

## Unidades

Las unidades (metro, caja, pieza…) que se pueden asignar a los productos.

1. En el menú, entra a **Unidades**. Arriba ves la lista actual.
2. En **Nueva unidad**, escribe el **Código** (corto, p. ej. `caja`) y el **Nombre**
   (p. ej. `Caja`).
3. Haz clic en **Agregar**.

Las unidades no se pueden editar ni eliminar desde la app.

## Familias

Una [[familia]] es una línea de producto (CCA, CCS & AC, ABASTILUM…) y cada una lleva
su propia [[serie]] de folio (QA, QS, QI…) con sus *Términos y condiciones* del PDF.
**QL** es una familia especial que no tiene productos: solo sirve para cotizar
[líneas libres](cotizaciones.md#agregar-una-línea-libre).

### Crear una familia

1. En el menú, entra a **Familias**. Arriba ves la lista actual.
2. En **Nueva familia**, escribe el **Nombre**.
3. En **Serie**, escribe dos letras que empiecen con Q (p. ej. `QF`). Cada familia
   tiene una serie distinta. Es el prefijo de sus folios (`QF0001`).
4. Opcional: la **Descripción de la serie** es el texto que aparece junto al prefijo al
   elegir la serie en una cotización nueva (p. ej. *QF — Cobijas ignífugas*).
5. En **Términos y condiciones**, escribe un término por línea; se imprimen en el PDF.
   Si quieres que cada cotización de la serie pida un tiempo de entrega, escribe
   `{tiempo_de_entrega}` donde debe ir, p. ej. `Tiempo de entrega: {tiempo_de_entrega}`.
   El vendedor tendrá que escribirlo antes de [emitir](cotizaciones.md#tiempo-de-entrega).
6. Marca *"Sin productos de catálogo"* solo si la familia es para líneas libres, como QL:
   no aparecerá al dar de alta productos.
7. Haz clic en **Crear familia**.

La serie nueva aparece de inmediato en **Nueva cotización**.

### Cambiar los términos de una familia

1. En **Familias**, haz clic en **Editar términos** en la fila de la familia.
2. Cambia la descripción o los términos y haz clic en **Guardar**.

Los cambios se ven en los borradores y en las cotizaciones que se emitan de ahora en
adelante. Las cotizaciones ya emitidas conservan los términos con que se emitieron.

> Los términos de **QL** incluyen el tiempo de entrega (`{tiempo_de_entrega}`), así que sus
> cotizaciones lo piden. Si los editas, conserva esa línea para que se siga pidiendo.

El nombre y la serie de una familia no se pueden cambiar ni eliminar desde la app.

Mensajes de error:

| Mensaje | Qué hacer |
|---|---|
| *El nombre es obligatorio.* | Escribe el nombre de la familia. |
| *La serie son dos letras y empieza con Q, p. ej. QF.* | Escribe algo como `QF`. |
| *Ya existe una familia con este nombre.* | Usa otro nombre. |
| *Ya existe una familia con esta serie.* | Elige otra serie. |
