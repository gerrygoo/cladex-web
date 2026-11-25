# Leads

Un **lead** es una persona que pide precios sin que todavía haya un proyecto: alguien que
quiere conocer los precios de una familia, o un contacto nuevo que todavía no pide nada
concreto. Los leads tienen su propia lista y su propio seguimiento, y **no cuentan** en
Proyectos, Pronóstico ni en el Dashboard. Cuando el lead pide algo concreto para un
proyecto, lo [conviertes en proyecto](#convertir-un-lead-en-proyecto).

Todos ven todos los leads. Los avisos de seguimiento vencido le llegan solo al vendedor
responsable.

## Ver los leads

1. En el menú **Leads**, elige **Ver leads**.
2. La lista muestra primero los **Abiertos**, con el seguimiento más atrasado arriba. Una
   fecha vencida dice *toca darle seguimiento*; uno sin fecha dice *sin seguimiento*.
3. Arriba de la tabla puedes ver los **Perdidos**, los **Convertidos** o **Todos**, y
   **Ver solo los míos**.

Haz clic en el nombre del contacto para abrir el lead.

## Dar de alta un lead

1. En el menú **Leads**, elige **Nuevo lead**.
2. Escribe el **Nombre del contacto**.
3. Si trabaja en un **cliente que ya existe** (p. ej. otra persona de Sonepar), elígelo en
   la lista. Si no, escribe la **Empresa**, o déjala vacía si es una persona.
4. Escribe su **Teléfono** o su **Correo** (al menos uno).
5. Elige el **Vendedor responsable**: a él le llegan los avisos de seguimiento.
6. Opcional: el **Próximo seguimiento** y **Qué pidió**, que queda como el primer
   comentario del historial.
7. Haz clic en **Guardar lead**.

## Dar seguimiento a un lead

En la página del lead, en **Seguimiento y comentarios**:

1. Escribe un **Comentario**: qué pasó y cuál es el siguiente paso.
2. Cambia el **Próximo seguimiento** si hace falta.
3. Haz clic en **Guardar seguimiento**.

Si pasa el día de seguimiento sin que se registre nada, el vendedor responsable recibe
el aviso *Seguimiento vencido*, como con los proyectos (si lo tiene activado en
[Mi cuenta](acceso.md#recibir-avisos)). Cualquier comentario los detiene.

Para cambiar el contacto, la empresa o el vendedor, abre **Editar datos del lead**, haz
los cambios y haz clic en **Guardar datos**. Un cambio de vendedor queda en el historial.

## Marcar un lead como perdido

1. En la página del lead, abre **Marcar como perdido**.
2. Escribe el **Motivo** y haz clic en **Marcar como perdido**.

Ya no recibe avisos. Si vuelve a interesarse, haz clic en **Reabrir lead**.

## Convertir un lead en proyecto

Cuando el lead pide productos y cantidades para un proyecto real:

1. En la página del lead, en **Convertir en proyecto**, elige el **Tipo de cotización**.
2. Haz clic en **Convertir en proyecto**.

Se crea una [cotización](cotizaciones.md) nueva, a nombre del vendedor responsable, y te
lleva a ella. Si el lead no era de un cliente que ya existe, también se crea el
[cliente](clientes.md) con su empresa (o su nombre) y sus datos de contacto. El
[proyecto](proyectos.md) se abre al emitir la cotización, como cualquier otro; en él
dice de qué lead viene. El lead queda como *Convertido en proyecto*, con su historial.

## Listas de precios

Una **lista de precios** es un PDF con todos los productos de una familia, con su precio
al margen que elijas, su vigencia y los términos y condiciones de su tipo de cotización.
Los precios se calculan igual que en una cotización de ese tipo. Todo va en pesos (MXN).

- **CCA**: precio por metro.
- **CCS y Alumoclad**: precio por metro y su equivalente en kilos, con el precio por kilo
  de **Configuración del sistema**, como en sus cotizaciones.
- **Astro**: precio por caja. **Exceline y Alumbrado**: precio por pieza.

La cantidad de cada producto sale con su unidad (`1 m`, `1 pza`, `1 caja`). Si un
producto no tiene unidad en el catálogo, la página lo avisa y un administrador se la pone
en [Productos](productos.md).

Una lista vale lo que diga la vigencia de su tipo de cotización: **hasta el último día
del mes** en que se hace, salvo que un administrador le haya puesto un número de días
(ver [Administración](administracion.md)). El PDF se guarda tal
como salió: aunque después cambie el precio por kilo o un margen, la lista ya enviada no
cambia.

En el menú **Leads**, **Listas de precios** muestra todas las que se han hecho, con su
PDF. Las de un lead también aparecen en su página.

### Armar una lista de precios

1. Desde la página de un lead (sección **Listas de precios**) o desde **Leads → Listas de
   precios**, elige la **Familia (tipo de cotización)** y haz clic en **Armar lista de
   precios**.
2. Todos los productos de la familia salen con cantidad 1. Cambia la cantidad de los que
   quieras (p. ej. `1000` metros) para que la lista muestre ese total; pon `0` para
   quitar un producto.
3. Elige el **Margen**. Para uno que no está en la lista, elige **Otro margen** y escribe
   el porcentaje en **Otro margen (%)**, que aparece al elegirlo.
4. Si los términos de ese tipo llevan tiempo de entrega, escríbelo.
5. Revisa **Para** (desde un lead ya viene su nombre) y haz clic en **Recalcular** para
   ver los precios.
6. Haz clic en **Generar PDF**. Si es para un lead, queda en su historial (y cuenta como
   seguimiento) y en su sección **Listas de precios**, de donde descargas el PDF.
