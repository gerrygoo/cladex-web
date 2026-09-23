# Guía de uso — Cotizador Cladex

Instrucciones paso a paso para usar el cotizador. Cada página cubre los flujos completos de una
sección de la app, en el orden en que normalmente se hacen.

En cada pantalla de la app, el botón **?** junto al título abre en otra pestaña la
parte de esta guía que la explica.

## Contenido

| Página | Qué cubre |
|---|---|
| [Acceso y cuenta](acceso.md) | Iniciar sesión, cambiar tu contraseña, salir |
| [Clientes](clientes.md) | Dar de alta, buscar, editar y eliminar clientes |
| [Cotizaciones](cotizaciones.md) | Crear una cotización, agregar productos por SKU, líneas libres, guardar, emitir, descargar el PDF, revisar |
| [Productos](productos.md) | Buscar, dar de alta y editar productos; cómo se calcula cada precio; conversiones de unidades |
:::admin
| [Administración](administracion.md) | Tipo de cambio; márgenes; materiales; usuarios; unidades (solo administradores) |
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
| Usuarios, Unidades, Ajustes | — | ✓ |

Si no ves **Usuarios**, **Unidades** o **Ajustes** en el menú, tu cuenta es de vendedor.

## La página de inicio

Al entrar ves un resumen de todas las cotizaciones (de todos los vendedores, no solo
las tuyas):

- **Cotizaciones por etapa**: cuántas hay en [[borrador]], [[emitida]] y [[revisada]], la
  suma de sus totales y las 3 de mayor total en cada etapa. Haz clic en un folio para
  abrirla. El botón **Nueva cotización** lleva a [crear una](cotizaciones.md#crear-una-cotización).
- **Vendedores**: una fila por cada persona que ha creado cotizaciones, ordenadas por
  **Monto emitido** (la suma de sus cotizaciones emitidas vigentes). Cuenta a quien creó
  la cotización, sea vendedor o administrador. Una revisada ya no suma al monto emitido:
  su lugar lo toma la revisión cuando se emite.
- Abajo, **Compilado el …** indica cuándo se instaló la versión actual de la app.

Todos los montos están en pesos (MXN) con IVA incluido.

## Conceptos

[[Folio]], [[serie]], [[borrador]], [[emitida]], [[vigencia]]… Todos los términos
están en el [glosario](glosario.md).

<!--
Mantenimiento de esta guía: cualquier cambio a una pantalla o flujo de la app actualiza
la página correspondiente en el mismo commit. Los nombres de botones, campos y mensajes
se citan exactamente como aparecen en la app. La app sirve estas páginas en /ayuda; ver
internal/guia para las convenciones ([[término]], :::admin).
-->
