# Administración

Solo para cuentas con rol **Administrador**. Estas opciones están en el menú **Catálogos**,
debajo de Productos, Clientes y Proveedores: **Usuarios**, **Familias y tipos de cotización**,
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

### Datos de facturación en órdenes de compra

Debajo del **Emisor**, lo que las [órdenes de compra a proveedor](ordenes-de-compra.md)
piden al proveedor en su factura, en el bloque **Datos de facturación**:

- **Razón social**, con el régimen de capital (*CLADEX S.A. de C.V.*).
- **Domicilio fiscal**, en un renglón, tal como saldrá.
- **Uso de CFDI** (*G01 - Adquisición de mercancías*).

El **RFC** y el **Régimen fiscal** son los del Emisor. Haz clic en **Guardar datos de
facturación**; verás *"Datos de facturación de las órdenes de compra guardados."* Lo que
cambies aquí sale en los borradores y en las órdenes que se confirmen después; las ya
confirmadas no cambian. Si falta alguno de estos datos, una orden de compra no se puede
confirmar.

## Notificaciones

Los avisos que la app manda por su cuenta, por correo (y más adelante por WhatsApp). En
**Configuración del sistema**, **Notificaciones** dice si cada canal está *Encendido* o
*Apagado* y por qué. Las llaves de los proveedores se instalan en el servidor
([cómo](../NAS_OPERATIONS.md#secrets)).

Haz clic en *Ver las notificaciones y enviar una prueba* para ver todas, de la más nueva a
la más vieja. Puedes filtrarlas por **Estado**, **Canal** y **Tipo**. Los estados:

- *Por enviar*: en la cola, o esperando un reintento después de un error.
- *Enviando*: se está mandando en este momento.
- *Aceptada*: el proveedor la recibió para entregarla. No quiere decir que ya se leyó.
- *Fallida*: no se pudo enviar; el **Error** dice por qué.
- *Cancelada*: ya no se va a enviar.

Haz clic en la fecha de una notificación para ver lo que decía y su **Historial**: cada
intento y su resultado.

**Enviar una prueba**

1. Escribe tu correo en **Mi cuenta → Avisos** ([cómo](acceso.md#recibir-avisos)).
2. En **Notificaciones**, haz clic en **Enviarme una prueba**.
3. Recarga la página en unos segundos: la prueba debe estar *Aceptada*, y el correo
   *Prueba de avisos de Cladex* en tu bandeja (revisa también el correo no deseado).

**Si una notificación falla**

La app la reintenta sola varias veces (ver [trabajos en segundo
plano](#trabajos-en-segundo-plano)). Si aun así queda *Fallida*, corrige lo que dice el
error (por ejemplo, el correo del usuario en **Usuarios → Avisos**) y haz clic en
**Reintentar** en la notificación. Una que sigue *Por enviar* se puede **Cancelar**.

> En el [ambiente de pruebas](#ambiente-de-pruebas) solo se envían avisos a las
> direcciones autorizadas; las demás quedan *Canceladas* con esa razón.

## Trabajos en segundo plano

Algunas tareas la app las hace por su cuenta, sin que nadie esté en la página: por
ejemplo, enviar un aviso el día que vence un seguimiento. Cada una es un **trabajo**. Para
verlos, en **Configuración del sistema** haz clic en *ver los trabajos y sus errores*.

- **En cola y fallidos**: los que esperan su hora (*Pendiente*), el que se está haciendo
  (*En curso*) y los que no se pudieron hacer (*Fallido*), con el **Último error**.
- **Terminados recientemente**: los últimos 50 (*Hecho* o *Cancelado*), con el
  **Resultado**; si un trabajo no hizo nada, ahí dice por qué.

Si un trabajo falla, la app lo reintenta sola: al minuto, a los 5 minutos, a la media hora,
a las 2 horas. Después de 5 intentos queda como *Fallido* y ya no se reintenta solo.
**Reintentar ahora** lo vuelve a poner en la cola de inmediato (en uno fallido, con 5
intentos nuevos). Corrige antes lo que dice el error, si es algo que se arregla en la app.

Para comprobar que los trabajos se están ejecutando, haz clic en **Encolar trabajo de
prueba** y recarga la página: en unos segundos aparece en **Terminados recientemente** con
*El trabajo de prueba se ejecutó.*

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

## Elegir los avisos de un usuario

Cada aviso se prende por persona, y solo un administrador lo puede hacer. Todos empiezan
apagados.

1. En **Usuarios**, en la fila del usuario, haz clic en **Avisos**.
2. Revisa que tenga su **Correo** y marca **Correo** en **Recibir avisos por**. Sin eso no
   le llega ningún aviso, aunque lo prendas.
3. En **Avisos**, marca los que debe recibir y haz clic en **Guardar avisos**.

Los avisos que hay:

- **Seguimiento vencido de sus proyectos**: para el vendedor. Si pasa el día de seguimiento
  de un proyecto suyo y nadie registra un comentario ni una nueva fecha, el día hábil
  siguiente le llega un correo a las 9:00, otro a las 12:00 y otro a las 18:00; después,
  uno cada día hábil a las 18:00.
- **Seguimientos vencidos del equipo**: para quien revisa al equipo. El segundo día hábil
  sin seguimiento, a las 12:00, llega un correo por cada proyecto con el vendedor, el
  valor y el último comentario; después, cada 3 días hábiles.

Los fines de semana no se manda nada. Los avisos de un proyecto se detienen en cuanto
alguien registra un comentario o una nueva fecha de seguimiento, o el proyecto deja de ser
prospecto. Cada correo dice de qué proyecto se trata, su cliente y contacto, el total,
costo, utilidad y margen, el último comentario y trae el enlace al proyecto.

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

Las unidades no se pueden editar ni eliminar desde la app. Lo que sí se cambia en la
lista es la **clave de unidad SAT** de cada una (p. ej. `MTR`), que tiene que existir en
el catálogo del SAT. La usan las facturas de los productos que aún no tienen su propia
[clave de unidad](productos.md#datos-para-facturar-sat).

## Familias y tipos de cotización

Una [[familia]] es una línea de producto del catálogo (CCA, CCS & AC, ABASTILUM…): cada
producto pertenece a una. Un [[tipo de cotización]] es una clase de cotización: su
[[serie]] de folio (QA, QS, QI…), su numeración, sus *Términos y condiciones* y cómo se
ve su PDF. Al [crear una cotización](cotizaciones.md#crear-una-cotización) se elige su
tipo, y el buscador de productos de esa cotización muestra la familia del tipo.

Los dos están en una sola página: en el menú **Catálogos**, elige **Familias y tipos de
cotización**. Cada familia aparece con:

- **Productos**: *De catálogo*, o *Solo líneas libres* para **QL**, una familia especial
  sin productos: la usa el tipo QL, que solo cotiza
  [líneas libres](cotizaciones.md#agregar-una-línea-libre).
- Su **Proveedor predeterminado**, con un enlace a su ficha, o *Sin asignar*. Se asigna
  desde el proveedor, en [Familias que surte](proveedores.md#familias-que-surte); aquí es
  solo de consulta. QL dice *No aplica*: no tiene proveedor predeterminado.
- Debajo, la tabla de sus **tipos de cotización**: serie, nombre, siguiente folio,
  membrete y lo que hace diferente cada uno, con su enlace **Editar tipo**.

Si algún tipo de cotización no tiene familia, aparece al final, en **Tipos sin
familia**.

Las familias y los tipos de cotización no se eliminan desde la app.

### Crear una familia

1. En **Familias y tipos de cotización**, baja a **Nueva familia**.
2. Escribe el **Nombre** y haz clic en **Crear familia**. Aparece en la página, todavía
   sin tipo de cotización.
3. Para cotizar sus productos con una serie propia, crea después su
   [tipo de cotización](#crear-un-tipo-de-cotización).

### Editar una familia

Haz clic en **Editar familia**, debajo de sus datos. Ahí puedes cambiar:

- El **Nombre**, p. ej. de `CCS & AC` a `CCS`. Sus productos y sus tipos de cotización
  siguen siendo los mismos y muestran el nombre nuevo, igual que los borradores. Las
  cotizaciones ya emitidas no cambian: su PDF no lleva el nombre de la familia.
- La **Clave de producto SAT de respaldo** (8 dígitos, p. ej. `26121600`), opcional. La
  clave SAT va en cada producto, en sus
  [Datos para facturar](productos.md#datos-para-facturar-sat); la de la familia solo la
  usan las líneas libres y los productos que todavía no tienen la suya. En cada factura
  se puede cambiar. Confírmala con el contador.

Haz clic en **Guardar**. El nombre de un tipo de cotización (p. ej. *Cable CCS & AC*) es
aparte: se cambia [en el tipo](#cambiar-un-tipo-de-cotización).

Mensajes de error de las familias:

| Mensaje | Qué hacer |
|---|---|
| *El nombre es obligatorio.* | Escribe el nombre de la familia. |
| *Ya existe una familia con este nombre.* | Usa otro nombre. |
| *La clave SAT son 8 dígitos, p. ej. 26121600.* | Escribe solo los 8 dígitos. |
| *La clave SAT 12345678 no existe en el catálogo del SAT.* | Revisa la clave con el contador. |

### Crear un tipo de cotización

1. En **Familias y tipos de cotización**, debajo de la familia, haz clic en **Nuevo tipo
   de cotización para esta familia**. Se abre el formulario con esa familia ya elegida
   en **Familia de sus productos**; el buscador de productos de sus cotizaciones la
   muestra.
2. En **Serie**, escribe una Q seguida de 1 a 5 letras (p. ej. `QCCA`). Es el prefijo de
   sus folios (`QCCA0001`).
3. Opcional: el **Nombre** aparece junto a la serie al elegir el tipo (p. ej.
   *QCCA — Cable CCA*).
4. En **Términos y condiciones**, escribe un término por línea; se imprimen en el PDF.
   Si quieres que cada cotización pida un tiempo de entrega, escribe
   `{tiempo_de_entrega}` donde debe ir, p. ej. `Tiempo de entrega: {tiempo_de_entrega}`.
   El vendedor tendrá que escribirlo antes de [emitir](cotizaciones.md#tiempo-de-entrega).
   Igual con `{moneda}`: el vendedor elige MXN o USD y el texto pasa a *pesos mexicanos
   (MXN)* o *dólares americanos (USD)*, p. ej. `Precios en {moneda}, no incluyen IVA`.
5. Elige el **Membrete del PDF** y marca lo que aplique:
   - *Solo líneas libres*: sin productos de catálogo, como QL. No se puede cambiar
     después.
   - *Precio por kg*: el costo de sus productos es por kilo y se cotizan por metro, como
     QG. Sus productos piden el peso (kg/m).
   - *"Otro margen" como precio por kg del cobre*, como QS.
   - *PDF sin la dirección de Cladex*.
   - *Al final de la lista de cotizaciones*.
6. Haz clic en **Crear tipo**. Aparece de inmediato debajo de su familia y en **Nueva
   cotización**.

Una familia puede tener más de un tipo de cotización: repite estos pasos desde la misma
familia.

### Cambiar un tipo de cotización

1. En **Familias y tipos de cotización**, haz clic en **Editar tipo** en su fila, debajo
   de su familia. Arriba dice cuál será su siguiente folio.
2. Cambia lo que necesites y haz clic en **Guardar**. Ahí también se le cambia la
   familia en **Familia de sus productos**.

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

Mensajes de error de los tipos de cotización:

| Mensaje | Qué hacer |
|---|---|
| *La serie es una Q seguida de 1 a 5 letras, p. ej. QCCA.* | Escribe algo como `QCCA`, sin números ni espacios. |
| *Esa serie ya la tiene o la usó otro tipo de cotización; elige otra.* | Elige otra serie. |
