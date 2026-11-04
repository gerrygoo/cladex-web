# Proveedores

El catálogo de proveedores guarda los datos fiscales, de contacto y de pago de quienes le
venden a Cladex, y qué [[familia|familias]] surte cada uno. Es la base para las órdenes de
compra a proveedores, que todavía no se hacen desde la app.

Todos ven la lista y la ficha de cada proveedor. Solo un administrador puede darlos de
alta, editarlos, marcarlos como inactivos y eliminarlos; un vendedor no ve sus datos
bancarios.

## Buscar un proveedor

1. En el menú **Catálogos**, elige **Proveedores**.
2. Escribe en el cuadro *"Buscar por nombre, RFC, razón social o contacto…"*. La lista se
   filtra mientras escribes.
3. Para ordenar, haz clic en el encabezado de una columna (**Nombre comercial**, **RFC**,
   **Razón social**, **Familias que surte**, **Estado**). Para acotar más,
   [filtra por columna](README.md#filtrar-las-tablas) con el embudo de cada encabezado;
   en **Estado** puedes elegir *Activo* o *Inactivo*.

Un vendedor hace clic en **Ver** para abrir la ficha del proveedor: sus datos fiscales,
familias, condiciones comerciales, notas, contactos (con quién **Recibe O.C.**) y su
historial. Solo un administrador puede modificarlo.

:::admin
## Dar de alta un proveedor

1. En **Proveedores**, haz clic en **Nuevo proveedor**.
2. Llena lo obligatorio: **Nombre comercial**, **RFC** (12 caracteres si es persona moral,
   13 si es persona física), **Razón social** y la **Dirección fiscal** completa:
   **Calle**, **Número**, **Colonia**, **Municipio o alcaldía**, **Estado** y
   **Código postal** (5 dígitos).
3. Opcional:
   - **Régimen fiscal**, de la lista del SAT.
   - **Familias que surte**: marca las familias de las que este proveedor será el
     predeterminado (ver [familias que surte](#familias-que-surte)).
   - **Contactos**: **Nombre**, **Puesto**, **Teléfono** y **Email** de cada uno. Marca
     **Recibe O.C.** en quien deba recibir las órdenes de compra; pueden ser varios. Es
     solo informativo: la app no envía correos. Para otro contacto, haz clic en
     **Agregar otro contacto**.
   - **Condiciones comerciales**: **Días de crédito**, **Moneda** (MXN o USD, la que usa
     para facturar; es solo informativa, no se convierte nada), **Condiciones de pago** y
     **Tiempo de entrega habitual**.
   - **Datos bancarios**: **Banco**, **CLABE** (18 dígitos) y **Beneficiario**. Los
     vendedores no los ven.
   - **Notas**.
4. Haz clic en **Guardar**. Verás *"Proveedor guardado."*

### Si el proveedor podría estar duplicado

Antes de guardar, la app revisa si ya hay un proveedor parecido, con el mismo criterio
que en [clientes](clientes.md#si-el-cliente-podría-estar-duplicado): el mismo **RFC**, o
un nombre comercial o razón social igual o que contiene al otro completo. Si encuentra
alguno, muestra *"Este proveedor podría estar duplicado"* con cada uno y tres opciones:
**Ver este proveedor**, **Sí, darlo de alta de todos modos** o **Regresar a editar**.

## Familias que surte

Cada familia tiene **un solo** proveedor predeterminado, o ninguno; un proveedor puede
surtir varias. Se asignan desde el proveedor, en **Familias que surte**. Cada familia se
muestra con su nombre y sus tipos de cotización, p. ej. *CCA — Cable CCA (QA)*; si una
familia tiene varios tipos de cotización, se muestran todos. Junto a una familia que ya
es de otro proveedor dice *(ahora: …)*.

- **QL (Líneas libres)** no aparece: cada cotización QL puede venir de un mayorista
  distinto.
- Si marcas una familia que ya es de otro proveedor, al guardar aparece
  *"La familia CCA ya está asignada a … ¿Quieres reasignarla?"*. Con
  **Sí, reasignar y guardar** pasa a este proveedor y deja de ser del otro; con
  **Regresar a editar** no se guarda nada.
- Al desmarcar una familia, se queda sin asignar.

En **Catálogos** → [Familias](administracion.md#familias) se ve el proveedor
predeterminado de cada familia, o *Sin asignar*.

## Editar un proveedor

1. [Busca el proveedor](#buscar-un-proveedor) y haz clic en **Editar** en su fila.
2. Cambia lo necesario: datos fiscales, familias, contactos, condiciones o datos
   bancarios. Para quitar un contacto, marca **Quitar este contacto**.
3. Haz clic en **Guardar**.

## Marcar un proveedor como inactivo

Al final de la ficha del proveedor, en **Estado**, haz clic en **Marcar como inactivo**.
Sigue en el catálogo con sus familias y en la lista dice *Inactivo*. Para volver a
usarlo, haz clic en **Reactivar**.

## Eliminar un proveedor

1. Haz clic en **Eliminar** en su fila (o en **Eliminar proveedor** en su ficha) y
   confirma *"¿Seguro que quieres eliminar a Cable Anti Robo? No se puede deshacer."*
2. Verás *"Proveedor eliminado."*

No se puede eliminar un proveedor que todavía es el predeterminado de alguna familia (más
adelante, tampoco uno con órdenes de compra). En ese caso la app dice
*"No se puede eliminar a …"*, con las familias que lo impiden, y ofrece
**Marcar como inactivo**. Si le quitas sus familias y guardas, ya se puede eliminar.
:::

## Historial de un proveedor

Al final de la ficha de cada proveedor está la sección **Historial**, de lo más reciente a
lo más antiguo, igual que en [clientes](clientes.md#historial-de-un-cliente): quién lo dio
de alta y cuándo, y cada edición con quién la hizo, la fecha y la hora y cada dato que
cambió (*"RFC: … → …"*), los contactos agregados o quitados (*"Contacto agregado: …"*) y
las familias (*"Familia que surte agregada: CCA"*). Lo que cargó el sistema sin un usuario
aparece como *Sistema*. Un vendedor no ve en el historial los cambios a los datos
bancarios.

## Problemas comunes

| Mensaje | Qué hacer |
|---|---|
| *El nombre comercial es obligatorio.* | Escribe el nombre comercial. Igual con RFC, razón social y cada dato de la dirección fiscal. |
| *El RFC debe tener 12 caracteres (persona moral) o 13 (persona física), p. ej. CAR191126QX6.* | Revisa el RFC (los espacios y guiones no cuentan). |
| *El código postal son 5 dígitos, p. ej. 76060.* | Escribe solo los 5 dígitos. |
| *La CLABE son 18 dígitos y no coincide con su dígito verificador; revísala.* | Revisa la CLABE: tiene 18 dígitos y el último se calcula con los demás, así que un dígito mal copiado se detecta. |
| *Los días de crédito son un número entero, p. ej. 30.* | Escribe solo el número de días. |
| *Escribe el nombre del contacto.* | Un contacto con teléfono o email necesita nombre; o borra sus datos. |
| *Email inválido.* | Revisa el correo del contacto (debe llevar `@`). |
