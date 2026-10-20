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

**Antes:** el cliente tiene que existir. Si no está, [dalo de alta](clientes.md#dar-de-alta-un-cliente).

1. En el menú **Cotizaciones**, elige **Nueva cotización**.
2. En **Cliente**, elige al cliente de la lista (está en orden alfabético).
3. En **Serie de folio**, elige una:
   - **QA — Cable CCA**
   - **QS — Cable CCS & AC**
   - **QI — Alumbrado**
   - **QL — Líneas libres**: para cotizar solo líneas libres (productos que no están en
     el catálogo y no tienen SKU).

   Un administrador puede agregar más series (ver [Familias](administracion.md#familias));
   la lista siempre muestra las que existen.
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
   - Junto al buscador está la casilla *"Solo productos de la familia …"* (CCA para
     serie QA, CCS & AC para QS, ABASTILUM para QI; en QL no aparece, porque no tiene
     productos), **marcada de inicio**: solo
     aparecen productos de la familia de la serie. Desmárcala para ver o buscar en todo
     el catálogo; cualquier producto se puede agregar a cualquier cotización.
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

### Usar otro margen (QA, QS y QI)

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
margen**, escribe en cualquiera de los dos y el otro se actualiza al instante. El precio
no puede ser menor que el costo del material.

> La lista de márgenes la mantiene un administrador en Configuración del sistema. Si cambia el porcentaje
> de tu margen, tu borrador toma el valor nuevo; si lo retira, tienes que
> [elegir otro](#problemas-comunes) antes de emitir.

## Agregar una línea libre

Para conceptos que no están en el catálogo: un flete, un producto especial, un servicio.

1. En **Agregar líneas**, junto a *"¿No está en el catálogo?"*, haz clic en **Línea libre**.
   Aparece una línea nueva. (En **QL**, el botón está arriba de las líneas, debajo de la
   vigencia.)
2. Escribe la **Descripción**; así aparecerá en el PDF.
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
viñeta dentro de *Términos y condiciones*, justo después de *"Pago por adelantado para
colocar OC"* y en el orden en que las escribiste. Un texto largo se acomoda en varias
líneas bajo la misma viñeta; los renglones vacíos se ignoran. Si lo dejas vacío, el PDF
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

El PDF solo muestra descripción, cantidad, precio unitario y total: **nunca el costo ni
el margen**. En la cotización emitida ves el **Costo total**, la **Utilidad** y el
**Margen**, que también llegan al proyecto, al pronóstico y a la página de inicio.

## Descargar y enviar el PDF

1. Abre la cotización: en el menú **Cotizaciones**, elige **Ver cotizaciones** y haz clic
   en su folio.
2. Haz clic en **Descargar PDF**. Se abre en otra pestaña; desde ahí guárdalo o
   imprímelo.
3. Envíalo al cliente por tu medio habitual (correo, WhatsApp). La app no envía
   correos.

> **Logo del PDF.** Las cotizaciones de las series **QA — Cable CCA** y **QS — Cable
> CCS & AC** llevan arriba a la izquierda el logo de *copperclad*. Las de las demás series (**QI**, **QL** y las que agregue un
> administrador) llevan el logo de CLADEX.

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

## Actualizar el costo de una cotización emitida

Los costos cambian después de mandar la cotización. Si eres el vendedor de la cotización
o un administrador, la cotización emitida (también si ya fue revisada) tiene una sección
**Costos** con el costo de cada línea libre:

1. Cambia el costo de las líneas que cambiaron.
2. Haz clic en **Guardar costos**.

Cambian la **Utilidad** y el **Margen** de la cotización (y del proyecto); los precios, el
total y el PDF quedan como se emitieron. En QL cada línea necesita su costo; en las demás
series puedes dejarlo vacío. Cada cambio queda registrado.

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
