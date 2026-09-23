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
| [Administración](administracion.md) | Tipo de cambio, precio del cobre y margen; usuarios; unidades (solo administradores) |
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

## Conceptos

[[Folio]], [[serie]], [[borrador]], [[emitida]], [[vigencia]]… Todos los términos
están en el [glosario](glosario.md).

<!--
Mantenimiento de esta guía: cualquier cambio a una pantalla o flujo de la app actualiza
la página correspondiente en el mismo commit. Los nombres de botones, campos y mensajes
se citan exactamente como aparecen en la app. La app sirve estas páginas en /ayuda; ver
internal/guia para las convenciones ([[término]], :::admin).
-->
