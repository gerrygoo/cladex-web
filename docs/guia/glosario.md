# Glosario

Los términos que usa el cotizador. En el resto de la guía, los términos subrayados con
puntos muestran su definición al pasar el cursor encima (o al tocarlos en el celular).

## Folio

Identificador único de una cotización, p. ej. `QA0012`. Se asigna al crearla y no
cambia nunca.

Una revisión conserva el folio de la original y le agrega `-R1`, `-R2`… Ver
[revisar una cotización emitida](cotizaciones.md#revisar-una-cotización-emitida).

## Serie

El prefijo del folio: **QA** (Cable CCA), **QS** (Cable CCS & AC) o **QI**
(Alumbrado). Define la numeración y los *Términos y condiciones* del PDF; no limita qué
productos lleva la cotización.

Ver [¿qué serie elijo?](cotizaciones.md#crear-una-cotización).

## Familia

La línea de producto a la que pertenece cada producto del catálogo: CCA, CCS & AC o
ABASTILUM. El buscador de la cotización filtra por la familia de la serie de inicio.

## SKU

La clave única de un producto en el catálogo. Es lo que escribes para buscarlo al armar
una cotización.

## Estados

Una cotización pasa por estos estados, en este orden:

<div class="estados">
  <span class="estado">borrador</span>
  <span class="flecha">Emitir →</span>
  <span class="estado">emitida</span>
  <span class="flecha">Revisar →</span>
  <span class="estado">revisada</span>
</div>

Al revisar se crea una nueva cotización (`QA0012-R1`) en borrador.

## Borrador

Una cotización que todavía se puede editar. Sus precios se recalculan con el catálogo y
los ajustes vigentes cada vez que la abres.

## Emitida

Una cotización congelada: sus precios, tipo de cambio, términos y PDF ya no cambian.
Es la que se envía al cliente.

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

## Tipo de cambio

Pesos por dólar (USD/MXN). Convierte a pesos el precio de los productos con moneda USD.
Lo captura un administrador en Ajustes.

## Precio del cobre

Pesos por kilo de cobre. Los productos que se cotizan por peso (Cable CCS & AC) cuestan
peso × precio del cobre. Lo captura un administrador en Ajustes.

## Margen

La fracción del precio de venta que es utilidad (`0.35` = 35 %). Los productos que se
cotizan con costo (Cable CCA) cuestan costo ÷ (1 − margen). Lo captura un
administrador en Ajustes.
