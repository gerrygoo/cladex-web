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
| [Proyectos](proyectos.md) | Dar seguimiento después de emitir: etapas, probabilidad de cierre, historial |
| [Productos](productos.md) | Buscar, dar de alta y editar productos; cómo se calcula cada precio; conversiones de unidades |
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
| Cotizaciones, Clientes, Productos | ✓ | ✓ |
| Usuarios, Familias, Unidades, Configuración del sistema | — | ✓ |

Si en el menú **Catálogos** solo ves **Productos** y **Clientes**, tu cuenta es de vendedor.

## La página de inicio

Al entrar ves un resumen de todos los [[proyecto|proyectos]] (de todos los vendedores,
no solo los tuyos). Un proyecto se abre al emitir una cotización, así que los borradores
que nunca se emitieron no aparecen aquí:

- **Proyectos por etapa**: cuántos hay en cada etapa de [[prospecto]],
  [[O.C. recibida]], [[en entrega]] y [[cerrado]], la suma de sus totales y los 3 de
  mayor total en cada etapa. El monto de un proyecto es el total de su cotización
  vigente. Haz clic en un folio para abrir el proyecto, o en el nombre de una etapa
  para ver todos los que están en ella. Para
  [crear una](cotizaciones.md#crear-una-cotización), usa el menú **Cotizaciones** →
  **Nueva cotización**.
  Para mover un proyecto de etapa, mira [darle seguimiento](proyectos.md#dar-seguimiento-a-un-proyecto).
- En **Prospectos** verás además cuántos son [[relevante para pronóstico|relevantes para pronóstico]]
  y cuánto suman, y la [[probabilidad de cierre]] de cada uno de los tres que se listan.
- **Perdidos**: debajo de las etapas, cuántos proyectos se
  [marcaron como perdidos](proyectos.md#marcar-un-proyecto-como-perdido) y cuánto
  suman. Haz clic para verlos. No cuentan en ninguna etapa ni en la tabla de vendedores.
- **Vendedores**: una fila por cada persona con proyectos, ordenadas por
  **Monto para pronóstico** (la suma de sus prospectos relevantes para pronóstico).
  Junto están el **Monto en prospectos** (todos sus prospectos) y cuántos proyectos
  tiene en cada etapa. Cuenta a quien creó la cotización que abrió el proyecto, sea
  vendedor o administrador.
- Abajo, **Compilado el …** indica cuándo se instaló la versión actual de la app.

Todos los montos están en pesos (MXN) con IVA incluido.

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
