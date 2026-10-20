# Clientes

Toda cotización se hace para un cliente de esta lista, así que el cliente tiene que
existir antes de cotizarle.

## Dar de alta un cliente

1. En el menú **Catálogos**, elige **Clientes**.
2. Haz clic en **Nuevo cliente**.
3. Escribe el **Nombre**. Es el único campo obligatorio y es el nombre que aparece en
   las cotizaciones y en el PDF, así que escríbelo como el cliente espera verlo.
4. Opcional: **RFC**, **Contacto**, **Teléfono**, **Email**, **Dirección**, **Notas** y,
   en **Datos fiscales**, lo que se necesitará para facturarle:
   - **Razón social**: como aparece en su constancia de situación fiscal, **sin** el
     régimen de capital (escribe *GRUPO PEME*, no *GRUPO PEME S.A. de C.V.*). El
     **Nombre** de arriba es el que sale en las cotizaciones y puede ser el comercial.
   - **Código postal fiscal**: el de su domicilio fiscal.
   - **Régimen fiscal**: elígelo de la lista del SAT (p. ej. *601 - General de Ley
     Personas Morales*).
   - **Uso de CFDI predeterminado**: el que usa normalmente (p. ej. *G01 - Adquisición
     de mercancías*); en cada factura se podrá cambiar.

   Todos son opcionales: un prospecto puede no tenerlos. Se revisarán al facturar.
5. Haz clic en **Guardar**.

Verás *"Cliente guardado."* y te quedas en la ficha del cliente. Cualquier usuario,
administrador o vendedor, puede dar de alta clientes. También puedes darlo de alta sin
salir de una cotización nueva: ver [Crear una cotización](cotizaciones.md#crear-una-cotización).

### Si el cliente podría estar duplicado

Antes de guardar, la app revisa si ya hay un cliente parecido: con el mismo **RFC**
(sin importar espacios, guiones ni mayúsculas), o con un nombre igual o que contiene al
otro completo, sin contar mayúsculas, acentos, puntos, comas ni la razón social (*S.A.*,
*S.A. de C.V.*, *S. de R.L.*, *S. de R.L. de C.V.*, *S.A.P.I. de C.V.*, *S.C.*). Por
ejemplo, *Sonepar S.A. de C.V.* se parece a *SONEPAR*.

Si encuentra alguno, en lugar de guardar muestra *"Este cliente podría estar
duplicado"* con cada cliente parecido (nombre, RFC y contacto) y tres opciones:

- **Usar este cliente**: no crea nada. Si venías de una cotización nueva, regresas a ella
  con ese cliente elegido; si no, a la lista de clientes.
- **Sí, darlo de alta de todos modos**: crea el cliente nuevo.
- **Regresar a editar**: cierra el aviso y deja lo que escribiste, sin guardar.

Es solo un aviso: siempre puedes darlo de alta. Al editar un cliente no se revisa.

**Siguiente paso:** [crear una cotización para este cliente](cotizaciones.md#crear-una-cotización).

## Buscar un cliente

1. Entra a **Clientes**.
2. Escribe en el cuadro *"Buscar por nombre, RFC o contacto…"*. La lista se filtra
   mientras escribes.
3. Para ordenar, haz clic en el encabezado de una columna (**Nombre**, **RFC**,
   **Contacto**, **Teléfono**, **Email**). Otro clic invierte el orden.
4. Para acotar más, [filtra por columna](README.md#filtrar-las-tablas) con el embudo de cada encabezado.

## Editar un cliente

1. [Busca el cliente](#buscar-un-cliente).
2. Haz clic en **Editar** en su fila.
3. Cambia lo necesario y haz clic en **Guardar**.

Las cotizaciones ya emitidas no cambian: su PDF conserva los datos con que se emitió.

## Eliminar un cliente

1. [Busca el cliente](#buscar-un-cliente).
2. Haz clic en **Eliminar** en su fila y confirma *"¿Eliminar este cliente?"*.

Qué pasa al eliminar:

- El cliente desaparece de la lista y ya no se puede elegir para cotizaciones nuevas.
- Sus cotizaciones existentes se conservan y siguen apareciendo en **Cotizaciones**.
- No se puede deshacer desde la app. Si fue un error, avisa a quien administra el
  sistema: el registro no se borra y se puede recuperar.

## Problemas comunes

| Mensaje | Qué hacer |
|---|---|
| *El nombre es obligatorio.* | Escribe el nombre del cliente. |
| *Email inválido.* | Revisa el correo (debe llevar `@`) o deja el campo vacío. |
