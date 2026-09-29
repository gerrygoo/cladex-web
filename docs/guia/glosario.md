# Glosario

Los términos que usa el cotizador. En el resto de la guía, los términos subrayados con
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

Una cotización pasa por estos estados, en este orden:

<div class="estados">
  <span class="estado">borrador</span>
  <span class="flecha">Emitir →</span>
  <span class="estado">emitida</span>
  <span class="flecha">Interesó →</span>
  <span class="estado">Pipeline</span>
  <span class="flecha">OC →</span>
  <span class="estado">OC emitida</span>
  <span class="flecha">Entrega →</span>
  <span class="estado">entregada</span>
  <span class="flecha">Cobro →</span>
  <span class="estado">entregada y cerrada</span>
</div>

Desde **emitida**, si el cliente pide cambios, [Revisar](cotizaciones.md#revisar-una-cotización-emitida)
crea una nueva cotización (`QA0012-R1`) en borrador y la original pasa a **revisada**.
Cómo mover una cotización de etapa: [darle seguimiento](cotizaciones.md#dar-seguimiento-a-una-cotización).

## Borrador

Una cotización que todavía se puede editar. Sus precios se recalculan con el catálogo,
su margen y los ajustes vigentes cada vez que la abres.

## Emitida

Una cotización congelada: sus precios, margen, términos y PDF ya no cambian.
Es la que se envía al cliente.

## Pipeline

Una cotización emitida que el cliente ya consideró y cuya propuesta avanza. Solo pasa a
Pipeline cuando hay avance con el cliente; una que todavía se está revisando no cuenta,
para no comprometer el Pipeline.

## OC emitida

El cliente ya mandó su orden de compra y la enviamos al fabricante.

## Entregada

El cable ya se le entregó al cliente; falta facturar y cobrar.

## Cerrada

Entregada y cerrada: se entregó, se facturó, se cobró y se emitió el complemento de pago
(si aplica).

## Revisada

Una cotización emitida que fue reemplazada por una revisión (`QA0012-R1`). Se conserva
intacta, con su PDF original.

## Línea libre

Una línea de cotización que no sale del catálogo (un flete, un servicio, un producto
especial). Tú escribes su descripción y su precio.

## Vigencia

Los 30 días a partir de la emisión durante los cuales es válida una cotización. Aparece
en el PDF.

## IVA

16 % sobre el subtotal de la cotización.

## Material

Una materia prima con su costo por unidad, p. ej. *CCS 30%* a $160.00 por kg. Un
producto hecho de materiales (Cable CCS & AC) cuesta, por cada material, la cantidad que
lleva × su precio, y a ese costo se le aplica el margen. Los precios los captura un
administrador en Ajustes.

## Margen

La parte del precio de venta que es utilidad, como porcentaje (35 %). Todo producto del
catálogo cuesta costo ÷ (1 − margen), donde el costo incluye el de sus materiales. Cada cotización usa uno
de los márgenes de una lista con nombre (p. ej. *Estándar (12.34%)*) que mantiene un
administrador en Ajustes; el vendedor lo elige al armar la cotización, o escribe uno propio
(*Personalizado*), ya sea como porcentaje o como precio por kilo del cobre CCS.
