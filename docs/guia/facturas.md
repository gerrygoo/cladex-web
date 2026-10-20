# Facturas

La app timbra las facturas de los proyectos y sus comprobantes de pago con Facturoporti, el
proveedor de certificación (PAC) de Cladex. Una factura timbrada es un CFDI ante el SAT: ya
no se edita, solo se cancela o se sustituye. Crear, timbrar y cancelar facturas es **solo
para administradores**; cualquier usuario puede verlas y descargarlas en **Proyectos →
Facturas** o desde su proyecto.

Mientras la facturación no esté configurada en el servidor, el proyecto sigue pidiendo el
folio de la factura escrito a mano, como hasta ahora ([Registrar la factura y el
pago](proyectos.md#registrar-la-factura-y-el-pago)).

## Antes de facturar

Una sola vez, en **Configuración del sistema** (administradores):

1. En **Facturación → Emisor**, los datos de Cladex como están en su constancia de
   situación fiscal: RFC, razón social (sin «S.A. de C.V.»), régimen fiscal, código postal
   del lugar de expedición y las series (*F* para facturas y *P* para pagos, si no se
   cambian).
2. En **Familias**, la **clave de producto SAT** de cada familia (8 dígitos, confírmala con
   el contador). Las partidas de la factura empiezan con ella.
3. En **Unidades**, la **clave de unidad SAT** de cada unidad (*MTR* metro, *KGM* kilogramo y
   *H87* pieza ya vienen puestas).

Y para cada cliente que se va a facturar, sus **datos fiscales** en su ficha: RFC, razón
social, régimen fiscal, código postal fiscal y uso de CFDI
([Clientes](clientes.md#dar-de-alta-un-cliente)).

## Crear una factura

1. Abre el proyecto. Tiene que estar en **O.C. recibida**, con su orden de compra.
2. En **Facturación y pago**, haz clic en **Crear factura**.

La factura empieza como **borrador** con las partidas de la cotización emitida: cantidades,
precios e importes no cambian. Revisa y completa:

- **Uso de CFDI** (sale del cliente) y, si la orden es P.U.E., la **forma de pago** con que
  pagó el cliente. Una orden P.P.D. lleva forma de pago *99 - Por definir*.
- **Enviar por correo a**: Facturoporti manda el XML y el PDF a estas direcciones al
  timbrarla. Puedes dejarlo vacío y mandarla tú.
- En cada partida, la **clave de producto SAT**, la **clave de unidad SAT**, la unidad y la
  descripción.

Haz clic en **Guardar borrador** para seguir después, o en **Guardar y validar**. Si falta
algo, la página dice qué (*"Para validarla falta: …"*).

## Timbrar una factura

Validada, la factura queda **lista para timbrar**, con su serie y folio (p. ej. *F-12*).

1. Revisa las partidas y el total.
2. Haz clic en **Timbrar** y confirma.

Si sale bien queda **timbrada**: ves su **folio fiscal (UUID)** y el proyecto pasa a
**Facturado** solo (P.U.E.: queda pagado; P.P.D.: queda *Facturado de anticipo*). Para
cambiar algo antes de timbrar, usa **Regresar a borrador para corregir**.

### Si queda «Timbrado incierto»

Pasa cuando Facturoporti no contesta a tiempo o contesta algo que no se entiende: no se sabe
si se timbró. Haz clic en **Reintentar con la misma solicitud**. Es seguro: se manda
exactamente lo mismo, y a una solicitud idéntica Facturoporti responde con el timbre que ya
hizo, así que nunca se timbra dos veces.

### Si Facturoporti la rechaza

La factura queda **Rechazada** con el motivo que dio el SAT o Facturoporti (por ejemplo, un
RFC o régimen del cliente que no coincide con el SAT). No se timbró nada. Corrige el dato (en
la ficha del cliente o en el borrador), haz clic en **Regresar a borrador para corregir** y
valídala otra vez.

## Descargar el XML y el PDF

En una factura timbrada, **Descargar PDF** y **Descargar XML**. El XML es el documento
legal; el PDF es su representación impresa (con el código QR para verificarla en el SAT) y
siempre se genera a partir del XML.

## Registrar el pago de una factura P.P.D.

Cuando el cliente termina de pagar una factura P.P.D.:

1. En la factura timbrada, haz clic en **Crear comprobante de pago**.
2. Escribe la **fecha en que se recibió el pago** y la **forma de pago**. El monto es el
   total de la factura, en un solo pago.
3. **Guardar y validar**, y luego **Timbrar**.

Al timbrarlo, el proyecto queda **Pagado** y se puede cerrar.

## Corregir una factura timbrada

Una factura timbrada no se edita: se **sustituye**.

1. En la factura, haz clic en **Crear factura que la sustituye**. Sale un borrador con sus
   mismos datos.
2. Corrígelo, valídalo y tímbralo.
3. Regresa a la factura original y cancélala con el motivo **01 - Comprobante emitido con
   errores con relación**: la app indica sola cuál la sustituye.

## Cancelar una factura

En la factura timbrada, en **Corregir o cancelar**, elige el **motivo** y haz clic en
**Cancelar ante el SAT**. Según el caso, el SAT la cancela en el momento o espera a que el
receptor acepte (**Cancelación en espera**); en ese caso usa **Consultar el estado en el
SAT** más tarde. Si el receptor la rechaza, la factura sigue vigente.

> Una factura con un comprobante de pago timbrado no se cancela hasta cancelar antes el
> pago. Cancelar una factura no regresa el proyecto de etapa: si hace falta, un
> administrador lo regresa a mano.

## Facturas de prueba

En el [ambiente de pruebas](administracion.md#ambiente-de-pruebas) las facturas se timbran
con el servidor de pruebas de Facturoporti: dicen **Pruebas: sin validez fiscal** y su PDF
lleva la marca *SIN VALIDEZ FISCAL*. Nunca se confunden con las reales.
