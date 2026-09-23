# Administración

Solo para cuentas con rol **Administrador**. Estas opciones aparecen en el menú como
**Usuarios**, **Unidades** y **Ajustes**.

## Actualizar tipo de cambio, precio del cobre y margen

1. En el menú, entra a **Ajustes**.
2. Actualiza los valores que cambiaron, solo con números:
   - **Tipo de cambio (USD/MXN)**: pesos por dólar, p. ej. `18.50`. Se usa en los
     productos con moneda USD.
   - **Precio del cobre ($/kg)**: p. ej. `145.30`. Se usa en los productos que se
     cotizan por peso (Cable CCS & AC).
   - **Margen por defecto**: como fracción, `0.35` = 35 %. Se usa en los productos que
     se cotizan con costo (Cable CCA). Es margen sobre el precio de venta: el precio es
     costo ÷ (1 − margen), así que con `0.35` un costo de $65.00 se cotiza a $100.00.
3. Haz clic en **Guardar**.

Efecto de un cambio:

- **Inmediato** en las cotizaciones nuevas y en **todos los borradores** (toman los
  valores nuevos la próxima vez que se abren o recalculan).
- **Ninguno** en las cotizaciones emitidas: conservan el tipo de cambio y los precios
  con que se emitieron.

> Avisa a los vendedores cuando cambies estos valores: los precios de sus borradores van
> a cambiar. Ver [cómo se calcula el precio de un producto](productos.md#cómo-se-calcula-el-precio-de-un-producto).

## Cambiar el rol de un usuario

1. En el menú, entra a **Usuarios**.
2. En la fila del usuario, elige **Administrador** o **Vendedor**.
3. Haz clic en **Guardar rol**.

No puedes cambiar tu propio rol.

## Deshabilitar o habilitar un usuario

1. En **Usuarios**, busca la fila del usuario.
2. Haz clic en **Deshabilitar** y confirma *"¿Deshabilitar a este usuario?"*. El usuario
   ya no puede iniciar sesión; sus cotizaciones se conservan.
3. Para devolverle el acceso, haz clic en **Habilitar**.

No puedes deshabilitarte a ti mismo.

## Dar de alta un usuario o restablecer una contraseña

Esto **no** se hace desde la app, sino por línea de comandos en el servidor. Quien
administra el servidor sigue el procedimiento de
[NAS_OPERATIONS.md](../NAS_OPERATIONS.md). Por ejemplo, para dar de alta un vendedor:

```bash
ssh cladex-nas docker compose exec cladex /cladex user add <usuario> --name "Nombre Completo" --role vendedor
```

Para restablecer una contraseña se usa `user passwd <usuario>` de la misma forma; esto
también cierra las sesiones abiertas de ese usuario.

En ambos casos el comando genera una contraseña y la muestra **una sola vez** en
pantalla. Entrégasela al usuario por un medio privado y pídele que la
[cambie](acceso.md#cambiar-tu-contraseña) desde **Mi cuenta** en cuanto entre.

## Unidades

Las unidades (metro, caja, pieza…) que se pueden asignar a los productos.

1. En el menú, entra a **Unidades**. Arriba ves la lista actual.
2. En **Nueva unidad**, escribe el **Código** (corto, p. ej. `caja`) y el **Nombre**
   (p. ej. `Caja`).
3. Haz clic en **Agregar**.

Las unidades no se pueden editar ni eliminar desde la app.
