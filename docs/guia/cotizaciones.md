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
8. Cuando la propuesta avanza, [dale seguimiento a su proyecto](proyectos.md#dar-seguimiento-a-un-proyecto)
   hasta que se entregue y se cobre.

## Crear una cotización

1. En el menú **Cotizaciones**, elige **Nueva cotización**.
2. En **Cliente**, escribe parte del nombre, RFC o contacto: mientras escribes, debajo
   aparecen los clientes que coinciden (sin importar mayúsculas ni acentos), con su RFC o
   contacto. Haz clic en el que quieras, o muévete con las flechas y presiona Enter. Al
   hacer clic en el cuadro sin escribir nada ves todos, en orden alfabético.

   ¿No está? Elige **➕ Agregar nuevo cliente**, la primera opción de la lista. Se abre
   el formulario de [Nuevo cliente](clientes.md#dar-de-alta-un-cliente); al guardarlo
   regresas aquí con el cliente ya elegido (y el tipo, si ya lo habías elegido). Con
   **Cancelar** regresas sin crear nada.
3. En **Tipo de cotización**, elige uno. Cada tipo tiene su serie de folio:
   - **QA — Cable CCA**
   - **QS — Cable CCS & AC**
   - **QI — Alumbrado**
   - **QL — Líneas libres**: para cotizar solo líneas libres (productos que no están en
     el catálogo y no tienen SKU).

   Un administrador puede agregar tipos o cambiar su serie (ver
   [Tipos de cotización](administracion.md#tipos-de-cotización)); la lista siempre
   muestra los que existen.
4. Haz clic en **Crear cotización**.

Llegas a la página de la cotización (p. ej. *"Cotización QS0007"*) con
el nombre de la cotización arriba y el cliente debajo. Es un [[borrador]]: lo reconoces porque puedes editar las líneas. El folio ya quedó reservado.

> **¿Qué serie elijo?** La [[serie]] hace dos cosas: define el folio (cada serie lleva su
> propia numeración: QA0001, QA0002…) y el bloque de **Términos y condiciones** que se
> imprime en el PDF (moneda, tiempo de entrega, empaque, flete). Elige la serie cuyos
> términos correspondan a lo que vendes. La serie **no** limita los productos: una
> cotización QS puede llevar productos de cualquier familia. La serie **QL** no tiene
> productos de catálogo: sus cotizaciones se arman con [líneas libres](#agregar-una-línea-libre).

> El cliente y la serie no se pueden cambiar después, y los borradores no se pueden
> borrar. Si te equivocaste, crea otra cotización y deja la equivocada sin emitir.

## Agregar productos por SKU

Con la cotización en borrador:

1. Debajo de las líneas y los totales está la sección **Agregar líneas**. Ahí ya aparece
   la lista de productos de la familia de la serie, en orden de SKU, 10 por página, cada
   uno como botón:
   `SKU — Descripción (precio)`. No hace falta escribir nada para verlos.
   - Usa **« Anterior** y **Siguiente »** para moverte entre páginas; la línea
     *"Página 1 de 3 · 23 productos"* te dice cuántos hay.
   - Para acotar la lista, escribe el SKU o parte de la descripción (p. ej. `THW`) en
     el cuadro *"Buscar producto por SKU o descripción…"*. La lista se filtra
     mientras escribes y vuelve a la página 1.
   - Solo aparecen productos de la familia de la serie (CCA para serie QA, CCS & AC
     para QS, ABASTILUM para QI): una cotización no acepta productos de otra familia.
     Si un borrador viejo trae uno, esa línea muestra *"Este producto no es de la
     familia …"* y no cuenta en el total hasta que la quites.
2. Haz clic en el producto. Se agrega como una línea nueva, con cantidad 1 y su precio
   ya calculado. La lista se queda en la misma página y en el mismo lugar de la
   pantalla, así que puedes seguir haciendo clic en el siguiente producto.
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

### Usar otro margen (QA, QS, QI y QG)

Si ninguno de la lista te sirve, por ejemplo para dar un punto de descuento sin bajar
hasta el siguiente margen, elige **Otro margen** al final de la lista **Margen** y escribe
el porcentaje en **Margen (%)**: de 0 a 99.99, con hasta 2 decimales, p. ej. `19.8`.
Aplica a todos los productos de la cotización, igual que uno de la lista, y se calcula
igual: costo ÷ (1 − margen). Por ejemplo, costo $1,000 con Alto (20.28%) da $1,254.39; con
*Otro margen* de 19.28% da $1,238.85.

- Es solo de esta cotización: no se agrega a la lista ni lo ven otras cotizaciones.
- Con 100 o más, un número negativo o el campo vacío, no se puede guardar ni emitir.
- Al reabrir el borrador, la lista dice *Otro margen (19.8%)*. Una
  [revisión](#revisar-una-cotización-emitida) arranca con el mismo margen y lo puedes
  cambiar.
- Si vuelves a elegir uno de la lista, el campo desaparece y el margen propio se
  descarta.
- Al emitir, la cotización queda con el margen *Otro margen* y su porcentaje. El PDF no
  muestra ningún margen.

En las cotizaciones **QS** (CCS y AC), junto a **Margen** siempre ves el margen elegido
de dos maneras (con uno de la lista, solo de lectura): el porcentaje y el **Precio CCS
30% ($/kg)**, el precio de venta por kilo del cobre que ese margen implica (costo del
material ÷ (1 − margen); solo si el catálogo tiene el material *CCS 30%*). Con **Otro
margen**, escribe en cualquiera de los dos y el otro se actualiza al instante. Manda el
que escribiste:

- Si escribes el **margen**, aplica la regla de arriba (hasta 2 decimales) y el precio por
  kilo se calcula.
- Si escribes el **precio por kilo** (hasta 2 decimales, p. ej. `188.50`), ese precio se
  respeta exacto en las líneas y en el PDF. El margen se calcula con todos sus decimales:
  en pantalla lo ves redondeado a 2, p. ej. *Otro margen (12.47%)*, pero los precios
  salen del precio por kilo que escribiste. El precio debe ser mayor que el costo del
  material; si es igual o menor, no se puede guardar ni emitir.
- Una revisión arranca con el mismo precio por kilo. Si el costo del material cambió
  mientras tanto, el precio se queda y el margen se ajusta.

En las cotizaciones **QG** (Cable Alumoclad hilo de guarda) funciona igual, pero el
precio por kilo se mide contra el **costo por kg de los productos** de la cotización, no
contra un material: el campo se llama **Precio Alumoclad ($/kg)** (con el nombre de la
familia). Si escribes un precio por kilo, cada metro se vende exacto a ese precio × su
**Peso (kg/m)**. Por ejemplo, ALC-C7#8 cuesta $95.00/kg y pesa 0.3896 kg/m: a $110.55/kg el
metro sale en 110.55 × 0.3896 = **$43.07028** y el margen es 14.07 % (1 − 95 ÷ 110.55).
Si escribes el margen, el precio por kilo es costo ÷ (1 − margen): con 20 %, 95 ÷ 0.80 =
$118.75.

- El costo por kg es el de los productos que ya están en la cotización. Sin líneas
  todavía, es el de los productos de la familia.
- Solo hay precio por kilo si todos esos productos tienen el **mismo costo por kg**. Si
  no, solo puedes escribir el margen y junto a él ves el aviso *"El precio por kg solo
  está disponible cuando todos los productos de la cotización tienen el mismo costo por
  kg."*
- Si ya tenías un precio por kilo y agregas un producto con otro costo por kg, el campo
  del precio desaparece y todas las líneas se calculan con el margen que daba ese precio
  (el aviso te lo recuerda). El precio queda guardado: si quitas ese producto, vuelve el
  campo y las líneas salen otra vez del precio exacto.

Las líneas de una cotización **QS** muestran también su peso: **Cantidad (m)**, **Kg por
metro** (los kilos de material por metro del producto, con 4 decimales) y **Kg**
(cantidad × kg por metro, con 2 decimales). Se calculan solos y se actualizan al cambiar la
cantidad; no hay que capturar nada. Salen igual en el PDF. Una línea libre no tiene peso
y deja esas columnas vacías.

Las cotizaciones **QG** (Cable Alumoclad hilo de guarda) también se cotizan por metro, con
las mismas columnas en la app y en el PDF. Ahí el **Costo** del producto es por kilo y su
**Peso (kg/m)** es lo que lleva un metro: precio por metro = costo por kg × kg por metro ÷
(1 − margen). Por ejemplo, ALC-C7#8 a $95.00/kg con 0.3896 kg/m y margen de 10 %: 95 ×
0.3896 = 37.01 de costo por metro; 37.01 ÷ 0.90 = **$41.12** por metro. 1000 m son 389.60
kg y $41,124.44 (el total sale del precio sin redondear, igual que en QS). Un producto QG
sin peso no se puede cotizar: su línea muestra un aviso y la cotización no se guarda ni se
emite hasta que lo quites ([ver abajo](#problemas-comunes)).

> La lista de márgenes la mantiene un administrador en Configuración del sistema. Si cambia el porcentaje
> de tu margen, tu borrador toma el valor nuevo; si lo retira, tienes que
> [elegir otro](#problemas-comunes) antes de emitir.

## Agregar una línea libre

Para conceptos que no están en el catálogo: un flete, un producto especial, un servicio.

1. En **Agregar líneas**, junto a *"¿No está en el catálogo?"*, haz clic en **Línea libre**.
   Aparece una línea nueva. (En **QL**, el botón está arriba de las líneas, debajo de la
   vigencia.)
2. Escribe la **Descripción**; así aparecerá en el PDF. Con **Enter** pasas a un renglón
   nuevo, y el PDF respeta esos saltos de línea (también al pegar un texto que ya los
   trae). Negritas y viñetas automáticas no se conservan; si quieres viñetas, escríbelas
   como un guion o `•` al inicio del renglón. En la factura, la descripción va en un solo
   renglón.
3. Escribe el **Precio unitario** en pesos, sin signo `$` (p. ej. `123.45`).
4. Opcional: escribe su **Costo** debajo del precio. No sale en el PDF; sirve para que la
   línea cuente en la utilidad de la cotización. Sin costo, la línea no suma utilidad.
5. Ajusta la **Cantidad** si no es 1.

En **QL** la línea se arma distinto: ver [Costo y margen por línea en QL](#costo-y-margen-por-línea-en-ql).

## Quitar una línea

Haz clic en **Eliminar** en la fila de la línea. No pide confirmación.

> ¿Quitaste una línea por error? Recarga la página **sin guardar** y acepta el aviso del
> navegador de que hay cambios sin guardar: vuelves a lo último que guardaste.

## Guardar el borrador

Los cambios en las líneas y en el margen **no se guardan solos**. Junto a los botones,
debajo de los totales, siempre ves en qué estado está la cotización:

- *"Todos los cambios guardados."* (en verde): no hay nada pendiente.
- *"Recalculando…"*: el sistema está actualizando los precios.
- *"Hay cambios sin guardar."* (en ámbar): guarda antes de salir.

1. Haz clic en **Guardar borrador**.
2. Verás *"Borrador guardado."*

> Si intentas salir de la página o recargarla con cambios sin guardar, el navegador te
> pregunta antes. Si confirmas, pierdes los cambios desde la última vez que guardaste.

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
   guardado, no lo que está en pantalla. Mientras haya cambios sin guardar, debajo de
   **Descargar PDF** aparece el aviso *"El PDF muestra lo último guardado, sin los
   cambios de la pantalla."*
2. Haz clic en **Descargar PDF**. Se abre en otra pestaña.

> La vista previa **no es el documento final**: no tiene fecha de vigencia y sus precios
> pueden cambiar hasta que emitas. No se la envíes al cliente; emite primero.

## Emitir la cotización

Cuando la cotización está lista para enviarse al cliente:

1. Revisa las líneas y los totales.
2. Haz clic en **Emitir cotización**. Guarda las líneas que están en pantalla y emite en
   un solo paso: no hace falta guardar antes.
3. Confirma *"¿Emitir la cotización? Sus precios quedan congelados; para cambiarla
   después habrá que crear una revisión."*
4. Verás *"Cotización emitida."* y, en el resumen debajo del cliente, **Estado** *Emitida*
   y el enlace a su **Proyecto**.

Qué pasa al emitir:

- Se abre su [[proyecto]], como [[prospecto]] con [[probabilidad de cierre]] Inicial.
  Ahí se le [da seguimiento](proyectos.md#dar-seguimiento-a-un-proyecto). Si la cotización
  es una revisión, sigue en el proyecto de la original.
- Se congelan los precios, el margen y los términos y condiciones. La
  cotización ya no se puede editar; aunque después cambien los ajustes o el catálogo,
  lo que ves en la página y en el PDF sigue siendo lo que se emitió.
- El PDF queda definitivo. Cada vez que lo descargues tendrá los mismos precios,
  cliente, vendedor, fechas y términos con que se emitió, aunque después cambien los
  ajustes, el catálogo o los datos del cliente.
- Cada línea lleva su número de **Partida** (1, 2, 3…), en la página y en el PDF, para
  referirte a ella en comentarios. En un borrador se renumeran si quitas o agregas
  líneas. Las cotizaciones emitidas antes de este cambio también salen numeradas al
  volver a descargarlas; nada más cambia.
- La [[vigencia]] es el último día del mes en que la emites (emitida el 16/10, vence el
  31/10; emitida el 31/10, vence ese mismo día); aparece en el PDF y en la página. En
  la serie **QL** no es automática: tú escribes la [fecha de vigencia](#moneda-y-vigencia-en-ql).

> Para emitir, la cotización necesita al menos una línea, ninguna línea con error y un
> margen disponible. En las series cuyos términos incluyen el tiempo de entrega (como
> **QL**), también necesita el [tiempo de entrega](#tiempo-de-entrega); y en **QL**,
> además, la [moneda y la vigencia](#moneda-y-vigencia-en-ql).

### Tiempo de entrega

En las cotizaciones de la serie **QL** (y de cualquier serie cuyos términos lo pidan),
la página del borrador muestra el campo **Tiempo de entrega (obligatorio para emitir)**,
arriba de las líneas. Escríbelo como quieras que se lea en el PDF (p. ej. `5 días
hábiles`). Se imprime en los términos y condiciones: *"Tiempo de entrega: 5 días hábiles"*.

- **Guardar borrador** lo guarda; no es obligatorio para guardar.
- **Emitir cotización** no avanza si está vacío y muestra *"Escribe el tiempo de entrega
  para poder emitir la cotización."*
- Al emitir se congela junto con los demás términos. Una revisión empieza con el mismo
  tiempo de entrega.
- En la vista previa del PDF de un borrador sin tiempo de entrega aparece *"por
  definir"*.

### Moneda y vigencia en QL

Las cotizaciones de la serie **QL** no tienen buscador de productos: solo se arman con
[líneas libres](#agregar-una-línea-libre). Además del tiempo de entrega, piden:

- **Moneda (obligatoria para emitir)**: elige **MXN** o **USD**. Los precios de las
  líneas libres los escribes directamente en esa moneda; la aplicación **no convierte
  nada**. El PDF muestra *Subtotal, IVA y Total* (el IVA se suma en ambas monedas) con la
  moneda al lado, y el primer término dice *"Precios en pesos mexicanos (MXN), no incluyen
  IVA"* o *"Precios en dólares americanos (USD), no incluyen IVA"*.
- **Vigencia: válida hasta (obligatoria para emitir)**: la fecha hasta la cual es válida
  la cotización. Escríbela tú; no puede ser anterior a hoy. Aparece en el PDF como
  *Vencimiento*.

**Guardar borrador** guarda lo que hayas elegido aunque esté incompleto; **Emitir
cotización** muestra qué falta. Una revisión conserva la moneda, pero hay que escribir
de nuevo la vigencia.

**Términos y condiciones adicionales (opcional)**, debajo de la vigencia: escribe
condiciones propias de esta cotización, una por renglón. En el PDF, cada renglón es una
viñeta dentro de *Términos y condiciones*, después de todos los términos del tipo de
cotización y en el orden en que las escribiste. Un texto largo se acomoda en varias
líneas bajo la misma viñeta; los renglones vacíos se ignoran. Caben hasta 10,000
caracteres. Si lo dejas vacío, el PDF
queda igual. Una revisión los conserva.

### Costo y margen por línea en QL

QL no tiene un margen general: cada línea lleva el suyo. En cada línea escribe:

- **Costo unitario**: lo que te cuesta el producto (p. ej. `1500` o `1,500`).
- **Margen %**: el margen de esa línea como porcentaje, de 0 a 99.99 (`10` es 10%).

El **Precio unitario** se calcula solo, con margen sobre el precio de venta: costo ÷
(1 − margen), redondeado a centavos. Costo 1,500 con 10% da **1,666.67** (utilidad 166.67,
el 10% del precio), no 1,650. El **Total** es cantidad × precio unitario, y el subtotal, el
IVA y el total se calculan sobre esos precios. Costo y margen son obligatorios para
guardar o emitir; con un margen de 100 o más, la línea marca error.

El PDF solo muestra partida, descripción, cantidad, precio unitario y total: **nunca el
costo ni el margen**. En la cotización emitida ves el **Costo total**, la **Utilidad** y el
**Margen**, que también llegan al proyecto, al pronóstico y a la página de inicio.

### Cotización del proveedor en QL

Debajo de los términos adicionales, la sección **Cotización del proveedor** guarda la
cotización que te mandó el proveedor y en la que te basaste. Es opcional (no hace falta
para guardar ni para emitir) y solo para uso interno: **nunca aparece en el PDF** que se
le manda al cliente.

1. Escribe **Proveedor (opcional)** y **No. de cotización del proveedor (opcional)**.
2. En **Agregar archivos**, elige uno o varios PDF, JPG o PNG de hasta 10 MB cada uno.
3. Haz clic en **Guardar cotización del proveedor**.

Cada archivo aparece con su nombre, tipo, tamaño, quién lo subió y cuándo. Haz clic en
el nombre para abrirlo en otra pestaña, o en **Descargar**. Si un archivo no es PDF, JPG
ni PNG, o pesa más de 10 MB, no se sube ninguno de los que elegiste y te dice cuál fue
(*"No se subió ningún archivo: «…» no es PDF, JPG ni PNG."*).

El proveedor y el número se pueden corregir en cualquier momento, también después de
emitir. Mientras la cotización es borrador puedes **Eliminar** un archivo; ya emitida,
sus archivos quedan fijos, pero puedes seguir agregando más. Una
[revisión](#revisar-una-cotización-emitida) ve los mismos archivos y datos y puede
agregar más archivos; solo puede eliminar los que se subieron después de la última
emisión. El [proyecto](proyectos.md#ver-el-detalle-de-un-proyecto) muestra la misma
sección.

## Descargar y enviar el PDF

1. Abre la cotización: en el menú **Cotizaciones**, elige **Ver cotizaciones** y haz clic
   en su folio.
2. Haz clic en **Descargar PDF**. Se abre en otra pestaña; desde ahí guárdalo,
   compártelo o imprímelo. El archivo se llama con el folio de la cotización y el
   nombre del cliente, por ejemplo `QS0011_SONEPAR.pdf` (o `QS0025-R2_José
   Camposeco.pdf` si es una revisión). En una cotización emitida va el nombre del
   cliente tal como quedó impreso en ella.
3. Envíalo al cliente por tu medio habitual (correo, WhatsApp). La app no envía
   correos.

> **Logo del PDF.** Arriba a la izquierda, las cotizaciones de la serie **QA — Cable CCA**
> llevan el logo de *ConduCCAsa*; las de **QS — Cable CCS & AC**, el de *copperclad*; y
> las de **QG — Cable Alumoclad**, el de *Alumoclad*. Las de las demás series (**QI**,
> **QL** y las que agregue un administrador) llevan el logo de CLADEX. La dirección es la
> misma en todas.

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
- La revisión sigue en el mismo [[proyecto]], con la misma etapa y
  [[probabilidad de cierre]]. Mientras está en borrador, el proyecto cuenta con el total
  de ese borrador.
- Solo se puede revisar mientras el proyecto es [[prospecto]]. Cuando el cliente ya
  mandó su orden de compra, en lugar del botón **Revisar** verás *"Esta cotización ya
  no se puede revisar"*.
- Arriba del borrador de la revisión, **Proyecto** tiene el enlace a su proyecto.
- Una revisión de la `-R1` (ya emitida) será `-R2`, y así sucesivamente. Solo se puede
  revisar la versión emitida más reciente.
- No se puede deshacer: al hacer clic en **Revisar**, la original queda como revisada.

> **Importante:** la revisión es un borrador. Empieza con el mismo margen que la
> original, pero sus precios se recalculan con el catálogo, el porcentaje de ese margen
> y los ajustes **actuales**, no con los de la original. Revisa los precios antes de
> emitirla.

## Cambiar el costo de una cotización emitida

El costo de una cotización emitida no se edita. Si cambió, [revisa la
cotización](#revisar-una-cotización-emitida): en el borrador de la revisión cambia el
costo de las líneas y emítela.

Al emitir la revisión, el historial del proyecto anota solo qué costos cambiaron
respecto a la versión anterior y cuánto subió o bajó cada uno, por ejemplo *"Partida 2
«Cable»: costo unitario $1,000.00 → $1,250.00 (subió $250.00, 25.0%)."* También anota las líneas
nuevas o quitadas que tenían costo. Solo cuentan las líneas libres: el costo de los
productos de catálogo es el del catálogo.

## Duplicar una cotización

Para mandar la misma cotización a otro cliente (o otra vez al mismo) sin capturar las
líneas de nuevo. Funciona con cualquier cotización, en borrador, emitida o revisada, y
de cualquier serie.

1. Abre la cotización y haz clic en **Duplicar**, abajo, junto a **Descargar PDF**.
   También está en el proyecto, junto a la **Cotización vigente**.
2. Elige el cliente de la copia. Viene el de la original; escribe para buscar otro por
   nombre, RFC o contacto. ¿No está? Elige **➕ Agregar nuevo cliente**, la primera opción
   de la lista: llenas sus datos, y al guardarlo regresas a Duplicar con ese cliente elegido.
3. Haz clic en **Duplicar**. La app crea un borrador nuevo y te lleva a él. Arriba dice
   *"Copia de QA0023 creada."* Si le das **Cancelar**, no se crea nada.
4. Edítalo como cualquier borrador y [emítelo](#emitir-la-cotización) cuando esté
   listo.

Qué se copia y qué no:

- La copia tiene **folio nuevo** de la misma serie (de `QA0023` sale, p. ej., `QA0031`),
  no `-R1`: es una cotización aparte, no una [revisión](#revisar-una-cotización-emitida).
  La original no cambia.
- Se copian las líneas y cantidades, el margen (también **Otro margen**), el tiempo de
  entrega, la moneda y los términos adicionales.
- En **QA, QS, QI y QG** los precios se calculan con los **costos actuales** del
  catálogo y el margen de la copia, no con los de la original.
- En **QL** se copian el costo y el margen de cada línea, la vigencia y la
  [cotización del proveedor](#cotización-del-proveedor-en-ql) con sus archivos. Todo se
  puede editar.
- No se copian comentarios, seguimiento, probabilidad ni la O.C. La copia no aparece
  en **Proyectos** hasta que la emites, como cualquier cotización nueva.
- Tú quedas como responsable de la copia.
- Si duplicas un borrador, se copia lo último que guardaste.

## Ver el detalle de una cotización

En el menú **Cotizaciones**, elige **Ver cotizaciones** y haz clic en el folio de
cualquier cotización que no sea [[borrador]]. Debajo del nombre de la cotización y del cliente verás este resumen, y luego las líneas y los totales:

- **Estado** de la cotización: *Emitida*, o *Revisada* si una revisión la reemplazó.
- **Proyecto**: el enlace a su [[proyecto]] y la etapa en que está. El
  [seguimiento](proyectos.md#dar-seguimiento-a-un-proyecto) se lleva ahí.
- **Emitió**: quién la emitió.
- **Emitida** y **Vigencia**: las fechas. Todas las fechas se ven como `28/09/2026 - 16:42`,
  en la zona horaria de tu navegador (la vigencia es solo un día, sin hora).
- **Margen**: el margen con el que se cotizó, congelado al emitir. En QL, el margen de
  toda la cotización: su utilidad entre su subtotal.
- **Costo total** y **Utilidad**: cuánto cuesta y cuánto gana la cotización antes de IVA
  (el margen aplicado a las líneas de catálogo, más el precio menos el costo de cada
  línea libre que tenga costo; una línea libre sin costo no suma utilidad).

## Comentar una cotización

Las cotizaciones ya no tienen caja para escribir comentarios: todo el seguimiento se
anota desde el [proyecto](proyectos.md#comentar-un-proyecto). Al final de cada
cotización, en **Comentarios**, se leen los de esa cotización (los más nuevos arriba, con
quién los escribió y cuándo) y un enlace lleva al proyecto para agregar uno nuevo. Los
comentarios no se editan ni se borran.

## Reasignar o eliminar un borrador

:::admin
Solo un administrador, y solo en un borrador que nunca se emitió (todavía no tiene
proyecto). Al final de su página, en **Administración**, elige el **Vendedor responsable**
y haz clic en **Reasignar**, o haz clic en **Eliminar borrador** y confirma; se borra con
sus partidas y comentarios y no se puede deshacer. Una cotización ya emitida se reasigna
o se elimina desde su [proyecto](proyectos.md#reasignar-o-eliminar-un-proyecto).
:::

## Buscar una cotización

1. En el menú **Cotizaciones**, elige **Ver cotizaciones**.
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
| *Selecciona un tipo de cotización.* | No elegiste QA, QS, QI o QL. | Elige uno ([¿cuál?](#crear-una-cotización)). |
| *Cantidad inválida.* | La cantidad está vacía, es cero o negativa, o no es un número. | Escribe un número mayor que 0. |
| *Descripción obligatoria.* | Una línea libre no tiene descripción. | Escríbela, o quita la línea. |
| *Precio inválido.* | Una línea libre tiene el precio vacío o con letras o signos. | Escribe solo el número, p. ej. `123.45`. |
| *Elige un margen para esta cotización.* | La cotización no tiene margen (p. ej. una revisión de una cotización antigua). | [Elige uno](#elegir-el-margen) en **Margen**. |
| *El margen elegido ya no está disponible; elige otro.* | Un administrador retiró el margen de la cotización. | [Elige otro](#elegir-el-margen) en **Margen**. |
| *Elige un margen disponible para esta cotización.* | Aparece en las líneas del catálogo mientras la cotización no tiene un margen disponible. | Igual: elige un margen en **Margen**. |
| Al hacer clic en **Emitir cotización** la página se recarga con un mensaje sobre el margen. | El margen de la cotización no está disponible. | Elige un margen en **Margen** y vuelve a emitir. |
| *Este producto no tiene datos de precio configurados.* | El producto no tiene costo ni materiales. | [Corrige el producto](productos.md#editar-un-producto) o usa una [línea libre](#agregar-una-línea-libre). |
| *Este producto no tiene Peso (kg/m) y en esta serie el precio por metro sale de él. Quítalo de la cotización y captura su peso en Productos.* | En una cotización QG, el producto no tiene peso, y sin él no hay precio por metro. | Quita la línea, [captura el peso del producto](productos.md#editar-un-producto) y vuelve a agregarlo. |
| *Producto no encontrado.* | El producto se eliminó del catálogo después de agregarlo al borrador. | Quita la línea y agrega el producto correcto. |
| *esta cotización ya no es editable* | La cotización ya fue emitida (p. ej. la emitiste desde otra pestaña). | Ábrela de nuevo. Para cambiarla, usa [Revisar](#revisar-una-cotización-emitida). |
| Al hacer clic en **Emitir cotización** la página se recarga sin mensaje. | La cotización no tiene líneas. | Agrega al menos una línea. |
