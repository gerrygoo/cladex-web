# Guía de uso — Cotizador Cladex

Instrucciones paso a paso para usar el cotizador en
<https://cotizador.cladex.com.mx>. Cada página cubre los flujos completos de una
sección de la app, en el orden en que normalmente se hacen.

## Contenido

| Página | Qué cubre |
|---|---|
| [Acceso y cuenta](acceso.md) | Iniciar sesión, cambiar tu contraseña, salir |
| [Clientes](clientes.md) | Dar de alta, buscar, editar y eliminar clientes |
| [Cotizaciones](cotizaciones.md) | Crear una cotización, agregar productos por SKU, líneas libres, guardar, emitir, descargar el PDF, revisar |
| [Productos](productos.md) | Buscar, dar de alta y editar productos; cómo se calcula cada precio; conversiones de unidades |
| [Administración](administracion.md) | Tipo de cambio, precio del cobre y margen; usuarios; unidades (solo administradores) |

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

- **Folio** — identificador único de una cotización, p. ej. `QA0012`. Se asigna al
  crearla y no cambia nunca.
- **Serie** — el prefijo del folio: **QA** (Cable CCA), **QS** (Cable CCS & AC) o
  **QI** (Alumbrado). Define la numeración y el bloque de *Términos y condiciones* del
  PDF. No limita qué productos puede llevar la cotización.
- **Estados** de una cotización:

  ```mermaid
  stateDiagram-v2
      direction LR
      [*] --> borrador: Crear cotización
      borrador --> emitida: Emitir cotización
      emitida --> revisada: Revisar (se crea una -R1 en borrador)
  ```

  - **borrador** — editable. Los precios se recalculan con los ajustes vigentes.
  - **emitida** — congelada: precios, tipo de cambio, términos y PDF ya no cambian.
  - **revisada** — reemplazada por una revisión (`QA0012-R1`); se conserva intacta.
- **Vigencia** — 30 días a partir de la emisión.
- **IVA** — 16 % sobre el subtotal.

---

*Mantenimiento de esta guía:* cualquier cambio a una pantalla o flujo de la app
actualiza la página correspondiente en el mismo commit. Los nombres de botones, campos y
mensajes se citan exactamente como aparecen en la app.
