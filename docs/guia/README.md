# Manual de usuario

Instrucciones paso a paso para usar el cotizador. Cada página cubre los flujos completos de una
sección de la app, en el orden en que normalmente se hacen.

En cada pantalla de la app, el botón **?** junto al título abre en otra pestaña la
parte de este manual que la explica.

## Contenido

| Página | Qué cubre |
|---|---|
| [Acceso y cuenta](acceso.md) | Iniciar sesión, cambiar tu contraseña, cerrar sesión |
| [Clientes](clientes.md) | Dar de alta, buscar, editar y eliminar clientes |
| [Cotizaciones](cotizaciones.md) | Crear una cotización, agregar productos por SKU, líneas libres, guardar, emitir, descargar el PDF, revisar |
| [Proyectos](proyectos.md) | Dar seguimiento después de emitir: etapas, probabilidad de cierre, fechas, pronóstico, perdidos, historial |
| [Facturas](facturas.md) | Timbrar la factura de un proyecto y su comprobante de pago, descargar XML y PDF, sustituir y cancelar |
| [Productos](productos.md) | Buscar, dar de alta y editar productos; cómo se calcula cada precio; conversiones de unidades |
| [Documentos oficiales](documentos.md) | Ver y descargar fichas técnicas, presentaciones y material de capacitación |
:::admin
| [Administración](administracion.md) | Márgenes; materiales; usuarios; unidades; familias y series (solo administradores) |
:::
| [Glosario](glosario.md) | Qué significa cada término: folio, serie, estados, margen… |

**¿Primera vez?** El recorrido típico es: [iniciar sesión](acceso.md#iniciar-sesión) →
[dar de alta al cliente](clientes.md#dar-de-alta-un-cliente) →
[crear la cotización](cotizaciones.md#crear-una-cotización) →
[agregar productos](cotizaciones.md#agregar-productos-por-sku) →
[emitir](cotizaciones.md#emitir-la-cotización) →
[descargar el PDF](cotizaciones.md#descargar-y-enviar-el-pdf).

> Si ves un aviso amarillo arriba que dice *"JavaScript está desactivado"*, la app sigue
> funcionando, pero cada acción recarga la página completa. Activa JavaScript en tu
> navegador para que buscar y editar líneas sea instantáneo.

## Quién puede hacer qué

| Sección | Vendedor | Administrador |
|---|:---:|:---:|
| Cotizaciones | ✓ | ✓ |
| Dar de alta y editar Clientes | ✓ | ✓ |
| Eliminar Clientes | — | ✓ |
| Dar de alta Productos | ✓ | ✓ |
| Editar y eliminar Productos (con sus materiales y conversiones) | — | ✓ |
| Ver y descargar Documentos oficiales | ✓ | ✓ |
| Subir y eliminar Documentos oficiales | — | ✓ |
| Usuarios, Familias, Unidades, Configuración del sistema | — | ✓ |

Si en el menú **Catálogos** solo ves **Productos** y **Clientes**, tu cuenta es de vendedor.
Si intentas algo que solo puede hacer un administrador, la app lo bloquea con *"No tienes
permiso para hacer esto. Solo un administrador puede hacerlo."*

## La página de inicio

Al entrar ves la tabla **Vendedores**, con una fila fija para Emilio Flores, Rodolfo Flores y
Gerardo Galván Rosillo (siempre las tres, aunque estén en ceros). Solo cuenta los
**proyectos activos**: los [[prospecto|prospectos]] que siguen en seguimiento. Los que ya
tienen la orden de compra recibida (*Orden colocada 100%*) y los
[perdidos](proyectos.md#marcar-un-proyecto-como-perdido) no cuentan. Cada proyecto va con la
persona que creó la cotización que lo abrió. Para
[crear una cotización](cotizaciones.md#crear-una-cotización), usa el menú **Cotizaciones** →
**Nueva cotización**.

Las columnas, en orden:

1. **Vendedor**.
2. **Proyectos totales**: cuántos proyectos activos tiene.
3. **Pipeline total (sin IVA)**: la suma del precio sin IVA de esos proyectos.
4. **Margen pipeline (sin IVA)**: el porcentaje y, junto a él, la utilidad en pesos (por ejemplo,
   *18.0% · $799,000.00*). El porcentaje es la suma de utilidades entre la suma de precios sin IVA,
   no un promedio de porcentajes. *—* si no tiene proyectos.
5. **Monto para pronóstico (sin IVA)** y **Proyectos en pronóstico**: la suma y la cantidad de sus
   proyectos con probabilidad **Alta 75%** o más. Se ven en el
   [pronóstico](proyectos.md#revisar-el-pronóstico).
6. **Sin seguimiento** y **Monto sin seguimiento (sin IVA)**: los proyectos sin fecha de próximo
   seguimiento, o con una fecha de hoy o anterior, y lo que suman. Un proyecto deja de
   contar cuando guardas un seguimiento con comentario y una fecha de próximo seguimiento
   posterior a hoy.
7. **O.C. vencidas**: los proyectos cuya fecha de *O.C. esperada* ya pasó y todavía no
   tienen la orden de compra.

Haz clic en el número de **Proyectos totales**, **Sin seguimiento** o **O.C. vencidas** para ver esos
proyectos, del monto mayor al menor: folio, cliente y monto sin IVA (por ejemplo,
*QA0026 · Pruebas · $892,631.19*); cada folio abre el proyecto. Las filas van ordenadas por **Monto sin
seguimiento**, de mayor a menor. Si las columnas no caben, la tabla se desliza de lado y
la columna **Vendedor** se queda fija. Los montos están en pesos (MXN) sin IVA, igual que en
el pronóstico, y las utilidades salen de las cotizaciones (una línea libre suma utilidad solo
si tiene costo; en QL todas lo tienen).

Abajo, **Compilado el …** indica cuándo se instaló la versión actual de la app.

## Filtrar las tablas

Las listas de **Cotizaciones**, **Productos**, **Clientes** y **Usuarios** se pueden
filtrar por cualquier columna, además de buscar y ordenar:

1. Haz clic en el embudo junto al nombre de la columna.
2. Llena el filtro, que depende del tipo de columna:
   - **Texto** (folio, nombre, RFC…): escribe una parte; no importan las
     mayúsculas.
   - **Elección** (estado, rol, familia, unidad, y en Cotizaciones cliente y autor):
     marca uno o varios valores de la lista (se muestran las filas que coincidan con
     cualquiera de los marcados; sin marcar ninguno, no se filtra). Si hay demasiados clientes o autores para listarlos
     (más de 200), esas dos columnas se filtran escribiendo, como el texto.
   - **Número** (total, costo): pon un mínimo, un máximo o ambos.
   - **Fecha**: pon la fecha *Desde*, *Hasta* o ambas (los dos días cuentan, según la zona horaria de tu navegador).
3. Haz clic en **Aplicar**. El embudo de la columna filtrada se pinta de azul.

En una pantalla angosta (celular), la tabla se desplaza de lado dentro de su recuadro; el
filtro se abre sobre la tabla y se cierra con **Escape** o al hacer clic fuera de él.

Puedes combinar filtros de varias columnas, y siguen activos al buscar u ordenar. Para
quitar uno, abre su embudo y haz clic en **Limpiar**; **Quitar todos** (arriba de la
tabla) los quita todos. Arriba de la tabla siempre ves si hay filtros: *"Filtros activos
en la tabla."* o *"Sin filtros."* La dirección de la página guarda los filtros, así que puedes
guardarla o compartirla.

## Páginas largas

Las listas de **Cotizaciones**, **Productos**, **Clientes**, **Usuarios** y **Unidades**, y
los **Comentarios** de cada cotización, muestran 20 filas por página. Cuando hay más de 10 filas, debajo de la tabla verás
*"Página 1 de 3"* con los enlaces **← Anterior** y **Siguiente →**, y el selector
**Filas por página** (10, 20, 50 o 100). Al buscar, filtrar u ordenar vuelves a la
página 1; el tamaño de página que elegiste se conserva.

## Conceptos

[[Folio]], [[serie]], [[borrador]], [[emitida]], [[vigencia]]… Todos los términos
están en el [glosario](glosario.md).

<!--
Mantenimiento de este manual: cualquier cambio a una pantalla o flujo de la app actualiza
la página correspondiente en el mismo commit. Los nombres de botones, campos y mensajes
se citan exactamente como aparecen en la app. La app sirve estas páginas en /ayuda; ver
internal/guia para las convenciones ([[término]], :::admin). Hay pruebas que verifican
que los textos citados sigan coincidiendo con la app (internal/web/guia_test.go, ver
docs/TESTING.md).
-->
