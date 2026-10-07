# Glosario

Los términos que usa el cotizador. En el resto del manual, los términos subrayados con
puntos muestran su definición al pasar el cursor encima (o al tocarlos en el celular).

## Folio

Identificador único de una cotización, p. ej. `QA0012`. Se asigna al crearla y no
cambia nunca.

Una revisión conserva el folio de la original y le agrega `-R1`, `-R2`… Ver
[revisar una cotización emitida](cotizaciones.md#revisar-una-cotización-emitida).

## Serie

El prefijo del folio: **QA** (Cable CCA), **QS** (Cable CCS & AC), **QI**
(Alumbrado) o **QL** (Líneas libres); cada [[familia]] tiene la suya. Define la numeración y los *Términos y condiciones* del PDF; no limita qué
productos lleva la cotización.

Ver [¿qué serie elijo?](cotizaciones.md#crear-una-cotización).

## Familia

La línea de producto a la que pertenece cada producto del catálogo: CCA, CCS & AC o
ABASTILUM. El buscador de la cotización filtra por la familia de la serie de inicio.
**QL** es una familia especial sin productos: solo sirve para cotizar líneas libres.
Los administradores crean familias nuevas en [Familias](administracion.md#familias).

## SKU

La clave única de un producto en el catálogo. Es lo que escribes para buscarlo al armar
una cotización.

## Estados

Una cotización tiene tres estados:

<div class="estados">
  <span class="estado">borrador</span>
  <span class="flecha">Emitir →</span>
  <span class="estado">emitida</span>
  <span class="flecha">Revisar →</span>
  <span class="estado">revisada</span>
</div>

Al emitirla se abre su [[proyecto]], que es el que avanza por etapas:

<div class="estados">
  <span class="estado">prospecto</span>
  <span class="flecha">OC →</span>
  <span class="estado">O.C. recibida</span>
  <span class="flecha">Entrega →</span>
  <span class="estado">en entrega</span>
  <span class="flecha">Cobro →</span>
  <span class="estado">cerrado</span>
</div>

Un proyecto que no se concreta se marca como [[perdido]]. La etapa se ve y se cambia en
la [página del proyecto](proyectos.md). Mientras el proyecto es prospecto, si el cliente pide cambios,
[Revisar](cotizaciones.md#revisar-una-cotización-emitida) crea una nueva cotización
(`QA0012-R1`) en borrador y la original pasa a **revisada**; el proyecto sigue siendo el
mismo. Cómo mover un proyecto de etapa: [darle seguimiento](proyectos.md#dar-seguimiento-a-un-proyecto).

## Borrador

Una cotización que todavía se puede editar. Sus precios se recalculan con el catálogo,
su margen y los ajustes vigentes cada vez que la abres.

## Emitida

Una cotización congelada: sus precios, margen, términos y PDF ya no cambian.
Es la que se envía al cliente.

## Proyecto

El negocio que se abre al emitir una cotización: se le da seguimiento desde que el
cliente la recibe hasta que se entrega y se cobra. Lleva el folio de la cotización que
lo abrió (`QA0012`) y sigue siendo el mismo aunque la cotización se revise
(`QA0012-R1`, `QA0012-R2`…): su cotización vigente es siempre la más reciente.

## Prospecto

Un proyecto cuya cotización ya tiene el cliente y todavía no manda su orden de compra.
Es la primera etapa: se le da seguimiento y se anota su [[probabilidad de cierre]].

## Probabilidad de cierre

Qué tan cerca está un prospecto de convertirse en pedido. Se elige entre cinco pasos:
Inicial (10%), Baja (25%), Media (50%), Alta (75%) e Inminente (90%). Un proyecto nuevo
empieza en Inicial.

## Relevante para pronóstico

Un prospecto con probabilidad de cierre Alta o Inminente. Son los proyectos que el
equipo revisa cuando ve qué está por cerrar (lo que antes se llamaba "en pipeline"). No
se marca a mano: depende solo de la probabilidad.

## O.C. recibida

El cliente ya mandó su orden de compra. Desde este momento la cotización del proyecto
ya no se puede revisar.

## En entrega

El pedido ya va en camino o se le entregó al cliente; falta facturar y cobrar.

## Cerrado

Un proyecto que ya se entregó, se facturó, se cobró y tiene su complemento de pago (si
aplica).

## Perdido

Un proyecto que no se concretó: el cliente ya no va a comprar. Se marca con un motivo,
sale de las etapas activas y ya no cuenta para el pronóstico. Solo un administrador
puede reabrirlo.

## Revisada

Una cotización emitida que fue reemplazada por una revisión (`QA0012-R1`). Se conserva
intacta, con su PDF original.

## Línea libre

Una línea de cotización que no sale del catálogo (un flete, un servicio, un producto
especial). Tú escribes su descripción y su precio.

## Vigencia

Los 30 días a partir de la emisión durante los cuales es válida una cotización. Aparece
en el PDF. En las cotizaciones QL la vigencia es una fecha que escribe el vendedor.

## IVA

16 % sobre el subtotal de la cotización.

## Material

Una materia prima con su costo por unidad, p. ej. *CCS 30%* a $160.00 por kg. Un
producto hecho de materiales (Cable CCS & AC) cuesta, por cada material, la cantidad que
lleva × su precio, y a ese costo se le aplica el margen. Los precios los captura un
administrador en Configuración del sistema.

## Margen

La parte del precio de venta que es utilidad, como porcentaje (35 %). Todo producto del
catálogo cuesta costo ÷ (1 − margen), donde el costo incluye el de sus materiales. Cada cotización usa uno
de los márgenes de una lista con nombre (p. ej. *Estándar (12.34%)*) que mantiene un
administrador en Configuración del sistema; el vendedor lo elige al armar la cotización, o escribe uno propio
(*Personalizado*), ya sea como porcentaje o como precio por kilo del cobre CCS.
